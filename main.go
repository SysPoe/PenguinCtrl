package main

import (
	"encoding/hex"
	"errors"
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

// TODO(macro): App is a process-wide service locator (domain services + recovery flags + UIState).
// Split into a composition-root factory that returns typed collaborators (document session,
// playback stack, operator UI shell) so window_loop and settings callbacks stop reaching through
// one god object for unrelated concerns.
type App struct {
	Show          *show.ShowManager
	Playback      *playback.Engine
	Remote        *remote.Dispatcher
	Media         media.Host
	Settings      *config.Store
	OperatorLog   *operatorlog.Store
	Journal       *project.EditJournal
	Timecode      *timecode.Service
	Redundancy    *redundancy.Service
	CrashReporter *crashreport.Reporter
	Recovered     bool
	RecoveredPath string
	UI            UIState
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
		placement := application.Settings.Snapshot().OperatorWindow
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
	// TODO(micro): 2s shutdown wait is a magic deadline; name a const (e.g. operatorWindowShutdownWait).
	case <-time.After(2 * time.Second):
		log.Print("operator window stopped without a shutdown result")
		return 1
	}
	cleanExit = true
	return 0
}

// TODO(macro): newApp is the composition root mixed with runtime policy (authority handoff
// stop-all, settings fan-out reconfigure, support-bundle paths, UI provider adapters). Keep
// construction/wiring here; move handoff and "STOP before takeover" rules into redundancy or a
// dedicated app/settings policy type so SettingsPage only receives domain ports.
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
		Show:          showManager,
		Playback:      engine,
		Remote:        remotePort,
		Media:         mediaBackend,
		Settings:      settings,
		OperatorLog:   operatorEvents,
		Journal:       journal,
		Timecode:      timecodeInput,
		Redundancy:    spare,
		CrashReporter: reporter,
		Recovered:     hasRecovery,
		RecoveredPath: recovered.DocumentPath,
		UI: UIState{
			SettingsPage:   settingsPage,
			ProjectLibrary: project.NewLibrary(),
		},
	}
	settingsPage.SetAudioDeviceProvider(func() ([]ui.AudioDevice, error) {
		devices, err := application.Media.AudioDevices()
		// TODO(micro): On err, still builds result from (likely nil) devices and returns both; prefer early `return nil, err` (same as videoRouting).
		result := make([]ui.AudioDevice, len(devices))
		for i, device := range devices {
			result[i] = ui.AudioDevice{ID: device.ID, Name: device.Name, IsDefault: device.IsDefault}
		}
		return result, err
	})
	configureVideoRoutingSettings(settingsPage, application.Media)
	settingsPage.SetOnSaved(func() {
		current := settings.Snapshot()
		timecodeInput.Configure(timecodeConfig(current), current.TimecodeListenAddress)
		wasAuthority := spare.Status().Authority
		if err := spare.Configure(redundancyConfig(current)); err != nil {
			operatorEvents.Add(operatorlog.ShowStopping, "Warm-spare redundancy", err.Error(), show.CueID{}, "")
		}
		if wasAuthority && !spare.Status().Authority && (len(application.Playback.ActiveInstances()) > 0 || len(application.Playback.ActiveExecutions()) > 0) {
			application.Playback.StopAll()
			operatorEvents.Add(operatorlog.ShowStopping, "Warm-spare redundancy", "Command authority changed while cues were active; local outputs were stopped", show.CueID{}, "")
		}
		application.Playback.RefreshDurations()
		application.Media.SyncOutputs(application.Playback.OutputIDs())
		application.Media.RefreshAudioDeviceStatus()
		refreshVideoRouting(application.Media)
	})
	settingsPage.SetOnReopenOutputs(func() {
		application.Media.EnsureOutputs(application.Playback.OutputIDs())
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
			if len(application.Playback.ActiveInstances()) > 0 || len(application.Playback.ActiveExecutions()) > 0 {
				return errors.New("STOP all local cues before taking command authority")
			}
			if err := spare.RequestTakeover(); err != nil {
				return err
			}
			operatorEvents.Diagnostic("Warm-spare redundancy", "Command authority acquired", map[string]any{"nodeId": spare.Status().NodeID})
			return nil
		},
		func() error {
			if len(application.Playback.ActiveInstances()) > 0 || len(application.Playback.ActiveExecutions()) > 0 {
				return errors.New("STOP all local cues before releasing command authority")
			}
			if err := spare.ReleaseAuthority(); err != nil {
				return err
			}
			operatorEvents.Diagnostic("Warm-spare redundancy", "Command authority released for handoff", map[string]any{"nodeId": spare.Status().NodeID})
			return nil
		},
	)
	application.UI.TBContext = ui.TBContext{
		TopBar:         &application.UI.TopBar,
		TogglePreview:  application.Playback.TogglePreview,
		StopPreview:    application.Playback.StopPreview,
		ProblemsForCue: application.Playback.CueProblems,
	}
	return application, nil
}

func timecodeConfig(settings config.Settings) timecode.Config {
	return timecode.Config{
		Source: timecode.Source(settings.TimecodeSource), Policy: timecode.Policy(settings.TimecodePolicy),
		// TODO(micro): 500ms jump tolerance is a magic duration; name a const (e.g. defaultTimecodeJumpTolerance).
		FrameRate: settings.TimecodeFrameRate, JumpTolerance: 500 * time.Millisecond,
	}
}

func redundancyConfig(settings config.Settings) redundancy.Config {
	return redundancy.Config{
		Role: redundancy.Role(settings.RedundancyRole), NodeID: settings.RedundancyNodeID,
		ListenAddress: settings.RedundancyListenAddress, PeerAddress: settings.RedundancyPeerAddress,
		SharedKey: settings.RedundancySharedKey, InterlockPath: settings.RedundancyInterlockPath,
		// TODO(micro): 500ms heartbeat / 2500ms peer timeout are magic durations; name shared redundancy timing consts.
		HeartbeatInterval: 500 * time.Millisecond, PeerTimeout: 2500 * time.Millisecond,
	}
}

func newTheme() *material.Theme {
	th := material.NewTheme()
	th.Face = font.Typeface("Nirmala UI")

	th.Palette.Bg = palette.Background
	th.Palette.ContrastBg = palette.SurfaceRaised
	th.Palette.Fg = palette.TextSoft
	th.Palette.ContrastFg = palette.Text

	return th
}
