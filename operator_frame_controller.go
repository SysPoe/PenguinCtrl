package main

import (
	"context"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/health"
	"github.com/syspoe/cusus/internal/taskgroup"
	"github.com/syspoe/cusus/media"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/playback"
	"github.com/syspoe/cusus/preflight"
	"github.com/syspoe/cusus/project"
	"github.com/syspoe/cusus/redundancy"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/timecode"
	"github.com/syspoe/cusus/ui"
)

const (
	frameClockDiscontinuityGap = 3 * time.Second
	prewarmCandidateLimit      = 3
)

type operatorFrameConfig struct {
	playback             *playback.Engine
	media                media.Host
	timecode             *timecode.Service
	redundancy           *redundancy.Service
	settings             *config.Store
	show                 *show.ShowManager
	library              *project.Library
	events               *operatorlog.Store
	panel                *ui.OperatorPanel
	topBar               *ui.TopBar
	audioWarningSettings *widget.Clickable
	safetyResume         *widget.Clickable
	tasks                *taskgroup.Group
	postUI               func(context.Context, func()) bool
	health               *health.Monitor
	preflight            *preflight.Service
	showAudioSettings    func()
}

// operatorFrameController applies show-control side effects before layout. It
// owns edge-triggered warning and emergency-reset state across Gio frames.
type operatorFrameController struct {
	operatorFrameConfig
	lastFrameAt                        time.Time
	lastAudioWarning, lastVideoWarning string
	lastMixerUnderruns                 map[string]uint64
	emergencyResetting                 bool
	lastFingerprintError               string
}

func newOperatorFrameController(config operatorFrameConfig) *operatorFrameController {
	return &operatorFrameController{
		operatorFrameConfig: config,
		lastFrameAt:         time.Now(),
		lastMixerUnderruns:  make(map[string]uint64),
	}
}

func (c *operatorFrameController) Update(gtx layout.Context, now time.Time) []preflight.Check {
	if gap := now.Sub(c.lastFrameAt); gap > frameClockDiscontinuityGap {
		c.playback.LatchClockDiscontinuity(gap)
	}
	c.lastFrameAt = now
	c.media.Prewarm(c.playback.PreloadCandidates(prewarmCandidateLimit))
	c.handleOperatorRequests(gtx)

	audioWarning, videoWarning := c.media.AudioDeviceWarning(), videoRoutingWarning(c.media)
	c.observeWarnings(audioWarning, videoWarning)
	c.observeMixerMetrics()
	healthSnapshot := c.health.Snapshot()
	c.panel.SetHealth(operatorHealthState(healthSnapshot).String())
	showState, settingsState := c.show.ShowSnapshot(), c.settings.Snapshot()
	runtime := preflight.RuntimeReadiness{
		ObservedAt: healthSnapshot.Generated,
		FreshFor:   2 * readinessRefreshInterval,
		Checks:     healthPreflightChecks(healthSnapshot, showState, settingsState),
	}
	checks := c.preflight.Request(showState, settingsState, runtime, c.playback.CueProblems)
	c.lastFingerprintError = updateRedundancyFingerprint(c.redundancy, showState, settingsState, c.library.Files(""), redundancyPreflightReady(checks), c.lastFingerprintError, func(message string) {
		c.events.Add(operatorlog.ShowStopping, "Warm spare", "Could not calculate the production fingerprint: "+message, show.CueID{}, "")
	})
	for index := range checks {
		checks[index].Acknowledged = c.show.ProblemAcknowledged(checks[index].Fingerprint)
	}
	return checks
}

func (c *operatorFrameController) Warnings() (string, string) {
	return c.lastAudioWarning, c.lastVideoWarning
}

func (c *operatorFrameController) handleOperatorRequests(gtx layout.Context) {
	if c.audioWarningSettings.Clicked(gtx) && c.showAudioSettings != nil {
		c.showAudioSettings()
	}
	if c.safetyResume.Clicked(gtx) {
		if c.emergencyResetting {
			c.panel.SetStatus("E-STOP reset is still running; playback remains latched")
		} else {
			c.timecode.Coordinator().Acknowledge(true)
			c.playback.AcknowledgeSafetyLatch()
			c.panel.SetStatus("Playback re-armed after operator acknowledgement · press GO when ready")
		}
	}
	if c.topBar.TakeEmergencyStopRequest() {
		c.startEmergencyReset()
	}
	if c.topBar.TakeBlackoutRequest() {
		c.playback.BlackoutAll()
		c.panel.SetStatus("BLACKOUT asserted · Ctrl+Shift+B")
	}
}

func (c *operatorFrameController) startEmergencyReset() {
	c.emergencyResetting = true
	c.topBar.SetEmergencyResetting(true)
	c.playback.BeginEmergencyReset()
	c.panel.SetStatus("E-STOP asserted · force-stopping and reinitializing media outputs")
	c.events.Add(operatorlog.ShowStopping, "E-STOP", "Force-stopping and reinitializing media outputs", show.CueID{}, "")
	c.tasks.Go("emergency-media-reset", func(ctx context.Context) {
		err := c.media.EmergencyReset(ctx)
		c.postUI(ctx, func() {
			c.playback.CompleteEmergencyReset(err)
			c.emergencyResetting = false
			c.topBar.SetEmergencyResetting(false)
			if err != nil {
				c.panel.SetStatus("E-STOP reset failed · playback remains latched")
				return
			}
			c.media.SyncOutputs(c.playback.OutputIDs())
			c.panel.SetStatus("E-STOP reset complete · media outputs ready")
			c.events.Add(operatorlog.Info, "E-STOP", "Media outputs reinitialized and playback re-armed", show.CueID{}, "")
		})
	})
}

func (c *operatorFrameController) observeWarnings(audioWarning, videoWarning string) {
	if audioWarning != "" && audioWarning != c.lastAudioWarning {
		severity := operatorlog.Warning
		if len(preflight.AudioWarningAffectedCues(c.show.Snapshot(), audioWarning)) > 0 {
			severity = operatorlog.CueFailure
		}
		c.events.Add(severity, "Audio output", audioWarning, show.CueID{}, "")
	} else if audioWarning == "" && c.lastAudioWarning != "" {
		c.events.Diagnostic("Audio output", "Audio endpoint health restored", nil)
	}
	if videoWarning != "" && videoWarning != c.lastVideoWarning {
		severity := operatorlog.Warning
		if len(preflight.VideoWarningAffectedCues(c.show.Snapshot(), c.settings.Snapshot(), videoWarning)) > 0 {
			severity = operatorlog.CueFailure
		}
		c.events.Add(severity, "Video output", videoWarning, show.CueID{}, "")
	} else if videoWarning == "" && c.lastVideoWarning != "" {
		c.events.Diagnostic("Video output", "Video output health restored", nil)
	}
	c.lastAudioWarning, c.lastVideoWarning = audioWarning, videoWarning
}

func (c *operatorFrameController) observeMixerMetrics() {
	for _, metrics := range c.media.AudioMixerMetrics() {
		if previous := c.lastMixerUnderruns[metrics.EndpointID]; metrics.TotalUnderruns > previous {
			c.events.Diagnostic("Audio mixer", "Audio underrun count increased", map[string]any{
				"endpointId": metrics.EndpointID, "underruns": metrics.TotalUnderruns,
				"activeSources": metrics.ActiveSources, "lastCallback": metrics.LastCallback,
			})
		}
		c.lastMixerUnderruns[metrics.EndpointID] = metrics.TotalUnderruns
	}
}
