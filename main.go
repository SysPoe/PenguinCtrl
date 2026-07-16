package main

import (
	"encoding/hex"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/health"
	"github.com/syspoe/cusus/internal/buildinfo"
	"github.com/syspoe/cusus/internal/crashreport"
	"github.com/syspoe/cusus/media"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/playback"
	"github.com/syspoe/cusus/project"
	"github.com/syspoe/cusus/redundancy"
	"github.com/syspoe/cusus/remote"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/support"
	"github.com/syspoe/cusus/timecode"
	"github.com/syspoe/cusus/ui"
)

const (
	operatorWindowShutdownWait   = 2 * time.Second
	defaultTimecodeJumpTolerance = 500 * time.Millisecond
	defaultRedundancyHeartbeat   = 500 * time.Millisecond
	defaultRedundancyPeerTimeout = 2500 * time.Millisecond
)

type App struct {
	Document      DocumentServices
	Playback      PlaybackServices
	Operator      OperatorServices
	CrashReporter *crashreport.Reporter
	Recovery      RecoveryState
	UI            UIState
}

type DocumentServices struct {
	Show     *show.ShowManager
	Settings *config.Store
	Journal  *project.EditJournal
}

type PlaybackServices struct {
	Engine     *playback.Engine
	Remote     *remote.Dispatcher
	Media      media.Host
	Timecode   *timecode.Service
	Redundancy *redundancy.Service
}

type OperatorServices struct{ Log *operatorlog.Store }

type RecoveryState struct {
	Recovered bool
	Path      string
}

type UIState struct {
	TopBar               ui.TopBar
	CueList              ui.CueListState
	PlaybackSidebar      ui.PlaybackSidebar
	TBContext            ui.TBContext
	SettingsPage         *ui.SettingsPage
	ProjectLibrary       *project.Library
	ShowSettings         bool
	AudioWarningSettings widget.Clickable
	OperatorPanel        ui.OperatorPanel
}

func main() { os.Exit(runMain()) }

func runMain() (exitCode int) {
	reporter := crashreport.New("")
	defer func() {
		if value := recover(); value != nil {
			_ = reporter.Write("main", value, debug.Stack())
			log.Printf("fatal panic: %v", value)
			exitCode = 2
		}
	}()
	application, err := newApp(reporter)
	if err != nil {
		log.Print(err)
		return 1
	}
	if err := reporter.InstallFatalOutput(); err != nil {
		log.Printf("install fatal crash output: %v", err)
		return 1
	}
	cleanExit := false
	defer func() { reporter.CloseFatalOutput(cleanExit) }()
	runResult := make(chan error, 1)
	reporter.Go("operator-window", func() {
		placement := application.Document.Settings.Snapshot().OperatorWindow
		window := new(app.Window)
		window.Option(
			app.Title("CuSus "+buildinfo.Identity()),
			app.Size(unit.Dp(placement.Width), unit.Dp(placement.Height)),
			app.MinSize(unit.Dp(480), unit.Dp(320)),
		)
		runResult <- application.run(window)
	})
	app.Main()
	select {
	case err := <-runResult:
		if err != nil {
			log.Print(err)
			return 1
		}
	case <-time.After(operatorWindowShutdownWait):
		log.Print("operator window stopped without a shutdown result")
		return 1
	}
	cleanExit = true
	return 0
}

func newApp(reporter *crashreport.Reporter) (*App, error) {
	settings, err := config.Open("")
	if err != nil {
		return nil, err
	}
	reporter.SetDirectory(filepath.Join(filepath.Dir(settings.Path()), "crashes"))
	showManager := show.NewShowManager()
	journal, err := project.OpenEditJournal(filepath.Join(filepath.Dir(settings.Path()), "show-recovery.jsonl"))
	if err != nil {
		return nil, err
	}
	recovered, hasRecovery, err := journal.Recover()
	if err != nil {
		return nil, err
	}
	if hasRecovery {
		showManager.ReplaceShow(recovered.Show)
	}
	operatorEvents := operatorlog.NewStore()
	operatorEvents.SetLogPath(filepath.Join(filepath.Dir(settings.Path()), "operator-events.jsonl"))
	operatorEvents.SetContext(buildinfo.Identity(), func() string {
		digest, err := showManager.ShowSnapshot().Digest()
		if err != nil {
			return "invalid-show"
		}
		return hex.EncodeToString(digest[:8])
	})
	log.SetOutput(operatorEvents.Writer("Runtime"))
	remotePort := remote.NewDispatcher(settings)
	engine := playback.NewEngineWithRemote(showManager, settings, remotePort)
	timecodeInput := timecode.NewService(timecodeConfig(settings.Snapshot()), settings.Snapshot().TimecodeListenAddress)
	engine.SetTimeline(timecodeInput.Coordinator())
	timecodeInput.Coordinator().SetOnDiscontinuity(func(gap time.Duration) {
		if timecodeInput.Coordinator().Status().Policy == timecode.PolicyHold {
			engine.LatchClockDiscontinuity(gap)
		}
	})
	spare := redundancy.NewService(redundancyConfig(settings.Snapshot()))
	engine.SetOperatorLog(operatorEvents)
	engine.SetDurationProbe(func(source string) (int64, error) {
		return media.ProbeDurationMs(settings.Snapshot().FFmpegPath, source)
	})
	engine.SetMediaValidator(func(source string, cueType show.CueType) error {
		return media.ValidateSource(settings.Snapshot().FFmpegPath, source, cueType)
	})
	engine.Start()
	mediaBackend := media.NewManager(engine, settings)
	mediaBackend.SyncOutputs(engine.OutputIDs())
	settingsPage := ui.NewSettingsPage(settings)
	application := &App{
		Document: DocumentServices{Show: showManager, Settings: settings, Journal: journal},
		Playback: PlaybackServices{
			Engine: engine, Remote: remotePort, Media: mediaBackend,
			Timecode: timecodeInput, Redundancy: spare,
		},
		Operator:      OperatorServices{Log: operatorEvents},
		CrashReporter: reporter,
		Recovery:      RecoveryState{Recovered: hasRecovery, Path: recovered.DocumentPath},
		UI: UIState{
			SettingsPage:   settingsPage,
			ProjectLibrary: project.NewLibrary(),
		},
	}
	hasActivePlayback := func() bool {
		return len(application.Playback.Engine.ActiveInstances()) > 0 || len(application.Playback.Engine.ActiveExecutions()) > 0
	}
	authorityControl := redundancy.NewAuthorityControl(spare, hasActivePlayback, application.Playback.Engine.StopAll)
	settingsPage.SetAudioDeviceProvider(func() ([]ui.AudioDevice, error) {
		devices, err := application.Playback.Media.AudioDevices()
		if err != nil {
			return nil, err
		}
		result := make([]ui.AudioDevice, len(devices))
		for i, device := range devices {
			result[i] = ui.AudioDevice{ID: device.ID, Name: device.Name, IsDefault: device.IsDefault}
		}
		return result, nil
	})
	configureVideoRoutingSettings(settingsPage, application.Playback.Media)
	settingsPage.SetOnSaved(func() {
		current := settings.Snapshot()
		timecodeInput.Configure(timecodeConfig(current), current.TimecodeListenAddress)
		stopped, err := authorityControl.Configure(redundancyConfig(current))
		if err != nil {
			operatorEvents.Add(operatorlog.ShowStopping, "Warm-spare redundancy", err.Error(), show.CueID{}, "")
		}
		if stopped {
			operatorEvents.Add(operatorlog.ShowStopping, "Warm-spare redundancy", "Command authority changed while cues were active; local outputs were stopped", show.CueID{}, "")
		}
		application.Playback.Engine.RefreshDurations()
		application.Playback.Media.SyncOutputs(application.Playback.Engine.OutputIDs())
		application.Playback.Media.RefreshAudioDeviceStatus()
		refreshVideoRouting(application.Playback.Media)
	})
	settingsPage.SetOnReopenOutputs(func() {
		application.Playback.Media.EnsureOutputs(application.Playback.Engine.OutputIDs())
	})
	settingsPage.SetOnSupportBundle(func() (string, error) {
		directory := filepath.Dir(settings.Path())
		path := filepath.Join(directory, "support-"+time.Now().Format("20060102-150405.000")+".zip")
		err := support.Export(path, operatorEvents.DiagnosticSnapshot(), settings.Path(), filepath.Join(directory, "crashes"))
		if err == nil {
			operatorEvents.Add(operatorlog.Info, "Operator action", "Created redacted support bundle", show.CueID{}, "")
		}
		return path, err
	})
	settingsPage.SetRedundancyControl(
		func() string { return health.RedundancySummary(spare.Status()) },
		func() error {
			if err := authorityControl.RequestTakeover(); err != nil {
				return err
			}
			operatorEvents.Diagnostic("Warm-spare redundancy", "Command authority acquired", map[string]any{"nodeId": spare.Status().NodeID})
			return nil
		},
		func() error {
			if err := authorityControl.ReleaseAuthority(); err != nil {
				return err
			}
			operatorEvents.Diagnostic("Warm-spare redundancy", "Command authority released for handoff", map[string]any{"nodeId": spare.Status().NodeID})
			return nil
		},
	)
	application.UI.TBContext = ui.TBContext{TopBar: &application.UI.TopBar}
	application.UI.TBContext.TogglePreview = application.Playback.Engine.TogglePreview
	application.UI.TBContext.StopPreview = application.Playback.Engine.StopPreview
	application.UI.TBContext.ProblemsForCue = application.Playback.Engine.CueProblems
	return application, nil
}

func timecodeConfig(settings config.Settings) timecode.Config {
	return timecode.Config{
		Source: timecode.Source(settings.TimecodeSource), Policy: timecode.Policy(settings.TimecodePolicy),
		FrameRate: settings.TimecodeFrameRate, JumpTolerance: defaultTimecodeJumpTolerance,
	}
}

func redundancyConfig(settings config.Settings) redundancy.Config {
	return redundancy.Config{
		Role: redundancy.Role(settings.RedundancyRole), NodeID: settings.RedundancyNodeID,
		ListenAddress: settings.RedundancyListenAddress, PeerAddress: settings.RedundancyPeerAddress,
		SharedKey: settings.RedundancySharedKey, InterlockPath: settings.RedundancyInterlockPath,
		HeartbeatInterval: defaultRedundancyHeartbeat, PeerTimeout: defaultRedundancyPeerTimeout,
	}
}

func newTheme() *material.Theme {
	th := material.NewTheme()
	th.Face = font.Typeface("Arial, Nirmala UI, Ubuntu, Times New Roman")

	th.Palette.Bg = palette.Background
	th.Palette.ContrastBg = palette.SurfaceRaised
	th.Palette.Fg = palette.TextSoft
	th.Palette.ContrastFg = palette.Text

	return th
}
