package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"image"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/x/explorer"

	"github.com/syspoe/cusus/health"
	"github.com/syspoe/cusus/internal/taskgroup"
	"github.com/syspoe/cusus/media"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/playback"
	"github.com/syspoe/cusus/project"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/ui"
)

func (a *App) run(window *app.Window) error {
	topBar := &a.UI.TopBar
	playbackSidebar := &a.UI.PlaybackSidebar
	tbCtx := &a.UI.TBContext
	manager := a.Show
	settingsStore := a.Settings
	playbackEngine := a.Playback
	mediaManager := a.Media
	settingsPage := a.UI.SettingsPage
	projectLibrary := a.UI.ProjectLibrary
	operatorEvents := a.OperatorLog
	operatorPanel := &a.UI.OperatorPanel
	topBar.SetStatusSink(operatorPanel.SetStatus)
	audioWarningSettings := &a.UI.AudioWarningSettings
	var ops op.Ops
	windowFocused := true
	var operatorHandle uintptr
	operatorPlacementApplied := false
	th := newTheme()
	expl := explorer.NewExplorer(window)
	uiActions := make(chan func(), 16)
	tasks := taskgroup.New(context.Background(), 4)
	defer func() {
		if err := tasks.Close(3 * time.Second); err != nil {
			operatorEvents.Diagnostic("Shutdown", err.Error(), nil)
		}
	}()
	postUI := func(ctx context.Context, action func()) bool {
		select {
		case uiActions <- action:
			window.Invalidate()
			return true
		case <-ctx.Done():
			return false
		}
	}
	var documentMu sync.RWMutex
	var saveMu sync.Mutex
	currentShowPath := a.RecoveredPath
	lastSavedDigest := showDigest(manager.ShowSnapshot())
	if a.Recovered {
		lastSavedDigest = [sha256.Size]byte{}
		operatorPanel.SetStatus("Recovered unsaved show edits · save to confirm recovery")
		operatorEvents.Diagnostic("Edit recovery", "Recovered unsaved show edits", map[string]any{"documentPath": currentShowPath})
	}
	var suppressJournal bool
	var documentGuard ui.DocumentGuard
	var closeInterceptor windowCloseInterceptor
	closeRequests := make(chan struct{}, 1)
	var lastAudioOperatorWarning, lastVideoOperatorWarning string
	var emergencyResetting bool
	lastMixerUnderruns := map[string]uint64{}
	var safetyResume widget.Clickable
	lastFrameAt := time.Now()
	healthMonitor := newHealthService(func() []health.Component {
		documentMu.RLock()
		path, dirty := currentShowPath, showDigest(manager.ShowSnapshot()) != lastSavedDigest
		documentMu.RUnlock()
		return collectHealthComponents(playbackEngine, mediaManager, a.Timecode, a.Redundancy, settingsStore.Snapshot(), path, dirty)
	})
	defer healthMonitor.Close()
	defer a.Timecode.Close()
	defer a.Redundancy.Close()
	power := startPowerKeeper()
	defer power.Close()
	preflightService := newPreflightService()
	defer preflightService.Close()
	playbackEngine.SetPreflightGate(func(cue show.Cue) error { return preflightService.Gate(manager.ShowSnapshot(), cue) })
	playbackEngine.SetAuthorityGate(a.Redundancy.Gate)
	playbackEngine.SetRemoteAuthorityExecutor(a.Redundancy.WithAuthority)
	cacheMaintainer := project.StartCacheMaintainer(
		func() bool {
			return len(playbackEngine.ActiveInstances()) > 0 || len(playbackEngine.ActiveExecutions()) > 0
		},
		func() []string {
			current, settings := manager.ShowSnapshot(), settingsStore.Snapshot()
			var paths []string
			for _, cue := range current.Cues {
				paths = append(paths, cueMediaSources(cue, settings)...)
			}
			return paths
		},
		func() (uint64, uint64) {
			settings := settingsStore.Snapshot()
			return uint64(settings.CacheQuotaGB) << 30, uint64(settings.CacheReserveGB) << 30
		},
	)
	defer cacheMaintainer.Close()
	tbCtx.LoadWaveform = func(source string, completed func([]float32, int, int64, error)) {
		tasks.Go("waveform", func(ctx context.Context) {
			wave, err := media.ExtractWaveformContext(ctx, settingsStore.Snapshot().FFmpegPath, source)
			if err != nil {
				operatorEvents.Add(operatorlog.Recoverable, "FFmpeg waveform", err.Error(), show.CueID{}, "")
			}
			postUI(ctx, func() { completed(wave.Samples, wave.SampleRate, wave.DurationMs, err) })
		})
	}
	playbackEngine.SetOnChange(func() {
		window.Invalidate()
		mediaManager.SyncOutputs(playbackEngine.OutputIDs())
	})
	operatorEvents.SetOnChange(window.Invalidate)
	manager.SetOnChange(func() {
		snapshot := manager.ShowSnapshot()
		documentMu.RLock()
		path, suppressed, dirty := currentShowPath, suppressJournal, showDigest(snapshot) != lastSavedDigest
		documentMu.RUnlock()
		if !suppressed && dirty && a.Journal != nil {
			if err := a.Journal.RecordDirty(snapshot, path); err != nil {
				operatorEvents.Add(operatorlog.Recoverable, "Edit recovery", err.Error(), show.CueID{}, "")
			}
		}
		window.Invalidate()
		playbackEngine.RefreshDurations()
		mediaManager.SyncOutputs(playbackEngine.OutputIDs())
	})

	tbCtx.ProjectFiles = func(kind string) []ui.ProjectFile {
		files := projectLibrary.Files(kind)
		result := make([]ui.ProjectFile, len(files))
		for i, file := range files {
			result[i] = ui.ProjectFile{Name: file.Name, Path: file.Source}
		}
		return result
	}
	tbCtx.PickFile = func(kind string, extensions []string, selected func(path string)) {
		tasks.Go("pick-media", func(ctx context.Context) {
			file, err := expl.ChooseFile(extensions...)
			if err != nil {
				if !errors.Is(err, explorer.ErrUserDecline) {
					log.Printf("pick file: %v", err)
					operatorEvents.Add(operatorlog.Recoverable, "OS file picker", err.Error(), show.CueID{}, "")
				}
				return
			}
			defer file.Close()

			path := ""
			switch f := file.(type) {
			case *explorer.File:
				path = f.URI()
			case *os.File:
				path = f.Name()
			}
			if path == "" || selected == nil {
				return
			}
			entry, duplicate, err := projectLibrary.Add(path, kind)
			if err != nil {
				operatorEvents.Add(operatorlog.Recoverable, "Media library", err.Error(), show.CueID{}, "")
				postUI(ctx, func() { operatorPanel.SetStatus("Could not add file: " + err.Error()) })
				return
			}

			postUI(ctx, func() {
				selected(entry.Source)
				if duplicate {
					operatorPanel.SetStatus("Duplicate detected · using existing " + entry.Name)
				} else {
					operatorPanel.SetStatus("Added " + entry.Name)
				}
			})
		})
	}

	loadShow := func() {
		tasks.Go("open-show", func(ctx context.Context) {
			file, err := expl.ChooseFile(".cusus")
			if err != nil {
				if !errors.Is(err, explorer.ErrUserDecline) {
					operatorEvents.Add(operatorlog.Recoverable, "Open show", err.Error(), show.CueID{}, "")
					postUI(ctx, func() { operatorPanel.SetStatus("Open failed: " + err.Error()) })
				}
				return
			}
			defer file.Close()
			loadedPath := explorerPath(file)
			tmp, err := os.CreateTemp("", "cusus-open-*.cusus")
			if err == nil {
				_, err = io.Copy(tmp, file)
			}
			if tmp != nil {
				tmp.Close()
				defer os.Remove(tmp.Name())
			}
			if err != nil {
				operatorEvents.Add(operatorlog.Recoverable, "Open show", err.Error(), show.CueID{}, "")
				postUI(ctx, func() { operatorPanel.SetStatus("Open failed: " + err.Error()) })
				return
			}
			manifest, files, err := project.Load(tmp.Name())
			if err != nil {
				operatorEvents.Add(operatorlog.Recoverable, "Open show", err.Error(), show.CueID{}, "")
				postUI(ctx, func() { operatorPanel.SetStatus("Open failed: " + err.Error()) })
				return
			}
			postUI(ctx, func() {
				playbackEngine.StopAll()
				projectLibrary.Replace(files)
				documentMu.Lock()
				suppressJournal = true
				documentMu.Unlock()
				manager.ReplaceShow(manifest.Show)
				documentMu.Lock()
				currentShowPath = loadedPath
				lastSavedDigest = showDigest(manifest.Show)
				suppressJournal = false
				documentMu.Unlock()
				if a.Journal != nil {
					if err := a.Journal.MarkSaved(manifest.Show, loadedPath); err != nil {
						operatorEvents.Add(operatorlog.Recoverable, "Edit recovery", err.Error(), show.CueID{}, "")
					}
				}
				operatorPanel.SetStatus("Loaded " + documentName(loadedPath) + " · recovery journal on")
				operatorEvents.Diagnostic("Open show", "Show loaded and verified", map[string]any{"documentPath": loadedPath, "assets": len(files)})
			})
		})
	}

	var saveAsShow func(func(bool))
	var saveShow func(func(bool))

	saveAsShow = func(done func(bool)) {
		complete := func(ctx context.Context, success bool) {
			if done != nil {
				postUI(ctx, func() { done(success) })
			}
		}
		tasks.Go("save-show-as", func(ctx context.Context) {
			file, err := expl.CreateFile("show.cusus")
			if err != nil {
				if !errors.Is(err, explorer.ErrUserDecline) {
					operatorEvents.Add(operatorlog.Recoverable, "Save show", err.Error(), show.CueID{}, "")
					postUI(ctx, func() { operatorPanel.SetStatus("Save failed: " + err.Error()) })
				}
				complete(ctx, false)
				return
			}
			path := explorerPath(file)
			if closeErr := file.Close(); closeErr != nil {
				operatorEvents.Add(operatorlog.Recoverable, "Save show", closeErr.Error(), show.CueID{}, "")
				complete(ctx, false)
				return
			}
			if strings.TrimSpace(path) == "" {
				operatorEvents.Add(operatorlog.Recoverable, "Save show", "file picker did not return a filesystem path", show.CueID{}, "")
				complete(ctx, false)
				return
			}
			postUI(ctx, func() { operatorPanel.SetStatus("Saving and optimizing bundled media…") })
			snapshot := manager.ShowSnapshot()
			saveMu.Lock()
			manifest, err := saveShowAtPath(path, snapshot, settingsStore.Snapshot().FFmpegPath, func(progress project.SaveProgress) {
				status := formatSaveProgress(path, progress)
				postUI(ctx, func() { operatorPanel.SetStatus(status) })
			})
			saveMu.Unlock()
			if err != nil {
				operatorEvents.Add(operatorlog.Recoverable, "FFmpeg / save show", err.Error(), show.CueID{}, "")
				postUI(ctx, func() { operatorPanel.SetStatus("Save failed: " + err.Error()) })
				complete(ctx, false)
				return
			}
			documentMu.Lock()
			currentShowPath = path
			lastSavedDigest = showDigest(snapshot)
			documentMu.Unlock()
			if a.Journal != nil {
				if err := a.Journal.MarkSaved(snapshot, path); err != nil {
					operatorEvents.Add(operatorlog.Recoverable, "Edit recovery", err.Error(), show.CueID{}, "")
				}
			}
			postUI(ctx, func() {
				operatorPanel.SetStatus("Saved " + documentName(path) + " · recovery journal on · " + formatFileCount(len(manifest.Assets)))
				operatorEvents.Diagnostic("Save show", "Show archive published", map[string]any{"documentPath": path, "assets": len(manifest.Assets)})
			})
			complete(ctx, true)
		})
	}

	saveShow = func(done func(bool)) {
		documentMu.RLock()
		path := currentShowPath
		documentMu.RUnlock()
		if path == "" {
			saveAsShow(done)
			return
		}
		tasks.Go("save-show", func(ctx context.Context) {
			snapshot := manager.ShowSnapshot()
			postUI(ctx, func() { operatorPanel.SetStatus("Saving " + documentName(path) + "…") })
			saveMu.Lock()
			manifest, err := saveShowAtPath(path, snapshot, settingsStore.Snapshot().FFmpegPath, func(progress project.SaveProgress) {
				status := formatSaveProgress(path, progress)
				postUI(ctx, func() { operatorPanel.SetStatus(status) })
			})
			saveMu.Unlock()
			if err != nil {
				operatorEvents.Add(operatorlog.Recoverable, "Save show", err.Error(), show.CueID{}, "")
				postUI(ctx, func() { operatorPanel.SetStatus("Save failed: " + err.Error()) })
				if done != nil {
					postUI(ctx, func() { done(false) })
				}
				return
			}
			documentMu.Lock()
			lastSavedDigest = showDigest(snapshot)
			documentMu.Unlock()
			if a.Journal != nil {
				if err := a.Journal.MarkSaved(snapshot, path); err != nil {
					operatorEvents.Add(operatorlog.Recoverable, "Edit recovery", err.Error(), show.CueID{}, "")
				}
			}
			postUI(ctx, func() {
				operatorPanel.SetStatus("Saved " + documentName(path) + " · recovery journal on · " + formatFileCount(len(manifest.Assets)))
				operatorEvents.Diagnostic("Save show", "Show archive published", map[string]any{"documentPath": path, "assets": len(manifest.Assets)})
			})
			if done != nil {
				postUI(ctx, func() { done(true) })
			}
		})
	}

	performNew := func() {
		playbackEngine.StopAll()
		projectLibrary.Replace(nil)
		documentMu.Lock()
		suppressJournal = true
		documentMu.Unlock()
		manager.ReplaceShow(show.Show{})
		documentMu.Lock()
		currentShowPath = ""
		lastSavedDigest = showDigest(show.Show{})
		suppressJournal = false
		documentMu.Unlock()
		if a.Journal != nil {
			_ = a.Journal.MarkSaved(show.Show{}, "")
		}
		operatorPanel.SetStatus("New untitled show · recovery journal on")
	}
	performDocumentAction := func(action ui.DocumentAction) {
		switch action {
		case ui.DocumentActionNew:
			performNew()
		case ui.DocumentActionOpen:
			loadShow()
		case ui.DocumentActionClose:
			closeInterceptor.AllowAndClose()
		}
	}

	for {
		e := window.Event()
		if err := closeInterceptor.HandleEvent(e, func() {
			select {
			case closeRequests <- struct{}{}:
			default:
			}
			window.Invalidate()
		}); err != nil {
			operatorEvents.Add(operatorlog.Recoverable, "Close protection", err.Error(), show.CueID{}, "")
		}
		expl.ListenEvents(e)
		for {
			select {
			case action := <-uiActions:
				action()
			default:
				goto actionsDone
			}
		}
	actionsDone:
		select {
		case <-closeRequests:
			snapshot := manager.ShowSnapshot()
			documentMu.RLock()
			dirty := showDigest(snapshot) != lastSavedDigest
			documentMu.RUnlock()
			if documentGuard.Request(ui.DocumentActionClose, dirty) {
				closeInterceptor.AllowAndClose()
			}
		default:
		}

		switch e := e.(type) {
		case app.DestroyEvent:
			snapshot := manager.ShowSnapshot()
			documentMu.RLock()
			path, dirty := currentShowPath, showDigest(snapshot) != lastSavedDigest
			documentMu.RUnlock()
			if dirty && a.Journal != nil {
				if err := a.Journal.RecordDirty(snapshot, path); err != nil {
					operatorEvents.Add(operatorlog.Recoverable, "Edit recovery", err.Error(), show.CueID{}, "")
				}
			}
			if placement, ok := operatorWindowPlacement(operatorHandle); ok {
				settings := settingsStore.Snapshot()
				settings.OperatorWindow = placement
				if err := settingsStore.Update(settings); err != nil {
					operatorEvents.Add(operatorlog.Recoverable, "Operator window", "Could not persist window placement: "+err.Error(), show.CueID{}, "")
				}
			}
			if err := tasks.Close(3 * time.Second); err != nil {
				operatorEvents.Diagnostic("Shutdown", err.Error(), nil)
			}
			healthMonitor.Close()
			a.Timecode.Close()
			a.Redundancy.Close()
			playbackEngine.Close()
			mediaManager.Close()
			return e.Err

		case app.ConfigEvent:
			windowFocused = e.Config.Focused
			window.Invalidate()

		case app.ViewEvent:
			if handle := operatorViewHandle(e); handle != 0 {
				operatorHandle = handle
				if !operatorPlacementApplied {
					operatorPlacementApplied = true
					placement := settingsStore.Snapshot().OperatorWindow
					tasks.Go("operator-window-placement", func(context.Context) { applyOperatorPlacement(handle, placement) })
				}
			}

		case app.FrameEvent:
			now := time.Now()
			if gap := now.Sub(lastFrameAt); gap > 3*time.Second {
				playbackEngine.LatchClockDiscontinuity(gap)
			}
			lastFrameAt = now
			gtx := app.NewContext(&ops, e)
			gtx.Execute(op.InvalidateCmd{At: time.Now().Add(time.Second)})
			if warmer, ok := mediaManager.(interface{ Prewarm([]playback.Instance) }); ok {
				warmer.Prewarm(playbackEngine.PreloadCandidates(3))
			}
			if audioWarningSettings.Clicked(gtx) {
				a.UI.ShowSettings = true
				settingsPage.ShowAudioDevices()
			}
			if safetyResume.Clicked(gtx) {
				if emergencyResetting {
					operatorPanel.SetStatus("E-STOP reset is still running; playback remains latched")
				} else {
					a.Timecode.Coordinator().Acknowledge(true)
					playbackEngine.AcknowledgeSafetyLatch()
					operatorPanel.SetStatus("Playback re-armed after operator acknowledgement · press GO when ready")
				}
			}
			if topBar.TakeEmergencyStopRequest() {
				emergencyResetting = true
				topBar.SetEmergencyResetting(true)
				playbackEngine.BeginEmergencyReset()
				operatorPanel.SetStatus("E-STOP asserted · force-stopping and reinitializing media outputs")
				operatorEvents.Add(operatorlog.ShowStopping, "E-STOP", "Force-stopping and reinitializing media outputs", show.CueID{}, "")
				resetter, ok := mediaManager.(media.EmergencyResetter)
				if !ok {
					err := errors.New("media backend does not support emergency reset")
					playbackEngine.CompleteEmergencyReset(err)
					emergencyResetting = false
					topBar.SetEmergencyResetting(false)
					operatorPanel.SetStatus("E-STOP reset failed · " + err.Error())
				} else {
					tasks.Go("emergency-media-reset", func(ctx context.Context) {
						err := resetter.EmergencyReset(ctx)
						postUI(ctx, func() {
							playbackEngine.CompleteEmergencyReset(err)
							emergencyResetting = false
							topBar.SetEmergencyResetting(false)
							if err != nil {
								operatorPanel.SetStatus("E-STOP reset failed · playback remains latched")
								return
							}
							mediaManager.SyncOutputs(playbackEngine.OutputIDs())
							operatorPanel.SetStatus("E-STOP reset complete · media outputs ready")
							operatorEvents.Add(operatorlog.Info, "E-STOP", "Media outputs reinitialized and playback re-armed", show.CueID{}, "")
						})
					})
				}
			}
			if topBar.TakeBlackoutRequest() {
				playbackEngine.BlackoutAll()
				operatorPanel.SetStatus("BLACKOUT asserted · Ctrl+Shift+B")
			}
			audioWarning := mediaManager.AudioDeviceWarning()
			videoWarning := videoRoutingWarning(mediaManager)
			if audioWarning != "" && audioWarning != lastAudioOperatorWarning {
				severity := operatorlog.Warning
				if len(audioWarningAffectedCues(manager.Snapshot(), audioWarning)) > 0 {
					severity = operatorlog.CueFailure
				}
				operatorEvents.Add(severity, "Audio output", audioWarning, show.CueID{}, "")
			} else if audioWarning == "" && lastAudioOperatorWarning != "" {
				operatorEvents.Diagnostic("Audio output", "Audio endpoint health restored", nil)
			}
			if videoWarning != "" && videoWarning != lastVideoOperatorWarning {
				severity := operatorlog.Warning
				if len(videoWarningAffectedCues(manager.Snapshot(), settingsStore.Snapshot(), videoWarning)) > 0 {
					severity = operatorlog.CueFailure
				}
				operatorEvents.Add(severity, "Video output", videoWarning, show.CueID{}, "")
			} else if videoWarning == "" && lastVideoOperatorWarning != "" {
				operatorEvents.Diagnostic("Video output", "Video output health restored", nil)
			}
			for _, metrics := range mediaManager.AudioMixerMetrics() {
				if previous := lastMixerUnderruns[metrics.EndpointID]; metrics.TotalUnderruns > previous {
					operatorEvents.Diagnostic("Audio mixer", "Audio underrun count increased", map[string]any{
						"endpointId": metrics.EndpointID, "underruns": metrics.TotalUnderruns,
						"activeSources": metrics.ActiveSources, "lastCallback": metrics.LastCallback,
					})
				}
				lastMixerUnderruns[metrics.EndpointID] = metrics.TotalUnderruns
			}
			lastAudioOperatorWarning, lastVideoOperatorWarning = audioWarning, videoWarning
			healthSnapshot := healthMonitor.Snapshot()
			operatorPanel.SetHealth(operatorHealthState(healthSnapshot).String())
			showState, settingsState := manager.ShowSnapshot(), settingsStore.Snapshot()
			preflight := preflightService.Request(showState, settingsState, audioWarning, videoWarning, playbackEngine.RemoteHealth(), playbackEngine.CueProblems)
			a.Redundancy.UpdateFingerprint(buildRedundancyFingerprint(showState, settingsState, projectLibrary.Files(""), redundancyPreflightReady(preflight)))
			preflight = append(preflight, healthPreflightChecks(healthSnapshot)...)
			for i := range preflight {
				preflight[i].Acknowledged = manager.ProblemAcknowledged(preflight[i].Fingerprint)
			}
			a.handleCueListShortcuts(gtx)
			documentMu.RLock()
			dirty := showDigest(manager.ShowSnapshot()) != lastSavedDigest
			documentMu.RUnlock()
			if !documentGuard.Visible() {
				if topBar.TakeNewRequest() && documentGuard.Request(ui.DocumentActionNew, dirty) {
					performNew()
				}
				if topBar.TakeLoadRequest() && documentGuard.Request(ui.DocumentActionOpen, dirty) {
					loadShow()
				}
				if topBar.TakeSaveRequest() {
					saveShow(nil)
				}
				if topBar.TakeSaveAsRequest() {
					saveAsShow(nil)
				}
			}
			paint.Fill(gtx.Ops, th.Bg)

			layout.Stack{}.Layout(gtx,
				// Main application layout
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{
						Axis: layout.Vertical,
					}.Layout(gtx,
						// Top Bar
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return topBar.Layout(th, gtx, manager.HasSelectedCue(), a.UI.ShowSettings)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return operatorPanel.LayoutBar(th, gtx, operatorEvents, preflight)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							if a.UI.ShowSettings {
								return settingsPage.Layout(th, gtx)
							}
							if gtx.Constraints.Max.X < gtx.Dp(unit.Dp(850)) {
								return ui.Main(
									th, gtx, manager, playbackEngine, operatorEvents,
									operatorPanel.OverlayVisible() || documentGuard.Visible(),
									func() { tbCtx.EditSelectedCue(manager) },
									func(field string) { tbCtx.EditSelectedCueAt(manager, field) },
									tbCtx.MoveCueActive(),
									func(index int) { tbCtx.MoveSelectedCueBefore(manager, index) },
									func() { tbCtx.MoveSelectedCueToEnd(manager) },
									func(groupID show.GroupID) { tbCtx.MoveSelectedCueIntoGroup(manager, groupID) },
									func(groupID show.GroupID) { tbCtx.MoveSelectedCueBeforeGroup(manager, groupID) },
									func(groupID show.GroupID) { tbCtx.MoveSelectedCueAfterGroup(manager, groupID) },
								)
							}
							return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return playbackSidebar.Layout(th, gtx, manager, playbackEngine)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									width := gtx.Dp(unit.Dp(1))
									paint.FillShape(gtx.Ops, th.ContrastBg, clip.Rect{Max: image.Pt(width, gtx.Constraints.Max.Y)}.Op())
									return layout.Dimensions{Size: image.Pt(width, gtx.Constraints.Max.Y)}
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return ui.Main(
										th, gtx, manager, playbackEngine, operatorEvents,
										operatorPanel.OverlayVisible() || documentGuard.Visible(),
										func() { tbCtx.EditSelectedCue(manager) },
										func(field string) { tbCtx.EditSelectedCueAt(manager, field) },
										tbCtx.MoveCueActive(),
										func(index int) { tbCtx.MoveSelectedCueBefore(manager, index) },
										func() { tbCtx.MoveSelectedCueToEnd(manager) },
										func(groupID show.GroupID) { tbCtx.MoveSelectedCueIntoGroup(manager, groupID) },
										func(groupID show.GroupID) { tbCtx.MoveSelectedCueBeforeGroup(manager, groupID) },
										func(groupID show.GroupID) { tbCtx.MoveSelectedCueAfterGroup(manager, groupID) },
									)
								}),
							)
						}),
					)
				}),
				// Top Bar Submenus
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					if a.UI.ShowSettings {
						return layout.Dimensions{}
					}
					return tbCtx.Layout(th, gtx, manager)
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return topBar.LayoutFileMenu(th, gtx)
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layoutWarnings(th, gtx, windowFocused, audioWarning, videoWarning, playbackEngine.SafetyLatchReason(), audioWarningSettings, &safetyResume)
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return operatorPanel.LayoutOverlay(th, gtx, operatorEvents, preflight, func(cueID show.CueID, edit bool, field string) {
						for index, cue := range manager.Snapshot() {
							if cue.ID == cueID {
								manager.SelectCue(index)
								if edit && strings.HasPrefix(field, "settings.") {
									a.UI.ShowSettings = true
								} else if edit {
									tbCtx.EditSelectedCueAt(manager, field)
								}
								break
							}
						}
					}, func(fingerprint string) { manager.AcknowledgeProblem(fingerprint) }, func() {
						a.UI.ShowSettings = true
					}, func() { manager.MoveSelection(1) })
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return documentGuard.Layout(th, gtx)
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return topBar.LayoutEmergencyStopConfirmation(th, gtx)
				}),
			)
			switch documentGuard.TakeChoice() {
			case ui.DocumentChoiceSave:
				if documentGuard.BeginSave() {
					saveShow(func(success bool) {
						if action, ok := documentGuard.ResolveSave(success); ok {
							performDocumentAction(action)
						}
					})
				}
			case ui.DocumentChoiceDiscard:
				if action, ok := documentGuard.Discard(); ok {
					performDocumentAction(action)
				}
			case ui.DocumentChoiceCancel:
				documentGuard.Cancel()
				closeInterceptor.ResetRequest()
			}
			if topBar.TakePageRequest() {
				a.UI.ShowSettings = !a.UI.ShowSettings
				window.Invalidate()
			}
			e.Frame(gtx.Ops)
		}
	}
}
