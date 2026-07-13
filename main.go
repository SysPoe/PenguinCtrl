package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/explorer"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/internal/crashreport"
	"github.com/syspoe/cusus/media"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/playback"
	"github.com/syspoe/cusus/project"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/ui"
)

type App struct {
	Show          *show.Manager
	Playback      *playback.Engine
	Media         media.Backend
	Settings      *config.Store
	OperatorLog   *operatorlog.Store
	Journal       *project.EditJournal
	Recovered     bool
	RecoveredPath string
	UI            UIState
}

type UIState struct {
	TopBar               ui.TopBar
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
	defer func() {
		if value := recover(); value != nil {
			_ = crashreport.Write("main", value, debug.Stack())
			log.Printf("fatal panic: %v", value)
			exitCode = 2
		}
	}()
	application, err := newApp()
	if err != nil {
		log.Print(err)
		return 1
	}
	if err := crashreport.InstallFatalOutput(); err != nil {
		log.Printf("install fatal crash output: %v", err)
		return 1
	}
	cleanExit := false
	defer func() { crashreport.CloseFatalOutput(cleanExit) }()
	runResult := make(chan error, 1)
	crashreport.Go("operator-window", func() {
		window := new(app.Window)
		window.Option(
			app.Title("CuSus"),
			app.Size(unit.Dp(1300), unit.Dp(720)),
			app.MinSize(unit.Dp(1300), unit.Dp(720)),
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
	case <-time.After(2 * time.Second):
		log.Print("operator window stopped without a shutdown result")
		return 1
	}
	cleanExit = true
	return 0
}

func newApp() (*App, error) {
	settings, err := config.Open("")
	if err != nil {
		return nil, err
	}
	crashreport.SetDirectory(filepath.Join(filepath.Dir(settings.Path()), "crashes"))
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
	engine := playback.NewEngine(showManager, settings)
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
		Media:         mediaBackend,
		Settings:      settings,
		OperatorLog:   operatorEvents,
		Journal:       journal,
		Recovered:     hasRecovery,
		RecoveredPath: recovered.DocumentPath,
		UI: UIState{
			SettingsPage:   settingsPage,
			ProjectLibrary: project.NewLibrary(),
		},
	}
	settingsPage.SetAudioDeviceProvider(func() ([]ui.AudioDevice, error) {
		devices, err := application.Media.AudioDevices()
		result := make([]ui.AudioDevice, len(devices))
		for i, device := range devices {
			result[i] = ui.AudioDevice{ID: device.ID, Name: device.Name, IsDefault: device.IsDefault}
		}
		return result, err
	})
	configureVideoRoutingSettings(settingsPage, application.Media)
	settingsPage.SetOnSaved(func() {
		application.Playback.RefreshDurations()
		application.Media.SyncOutputs(application.Playback.OutputIDs())
		application.Media.RefreshAudioDeviceStatus()
		refreshVideoRouting(application.Media)
	})
	settingsPage.SetOnReopenOutputs(func() {
		application.Media.EnsureOutputs(application.Playback.OutputIDs())
	})
	application.UI.TBContext = ui.TBContext{
		TopBar:         &application.UI.TopBar,
		TogglePreview:  application.Playback.TogglePreview,
		StopPreview:    application.Playback.StopPreview,
		ProblemsForCue: application.Playback.CueProblems,
	}
	return application, nil
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

func (a *App) handleCueListShortcuts(gtx layout.Context) {
	topBar := &a.UI.TopBar
	playbackSidebar := &a.UI.PlaybackSidebar
	tbCtx := &a.UI.TBContext
	manager := a.Show
	playbackEngine := a.Playback
	if a.UI.ShowSettings || tbCtx.CueEditorOpen() {
		return
	}
	if topBar.AddCueMenuOpen() || topBar.ActionMenuOpen() || topBar.FileMenuOpen() {
		for {
			event, ok := gtx.Event(key.Filter{Name: key.NameEscape})
			if !ok {
				return
			}
			if event, ok := event.(key.Event); ok && event.State == key.Press {
				topBar.CloseMenus()
				return
			}
		}
	}
	if tbCtx.DeleteConfirmationOpen() {
		tbCtx.HandleDeleteConfirmationKeys(gtx, manager)
		return
	}
	if tbCtx.MoveCueActive() {
		for {
			event, ok := gtx.Event(key.Filter{Name: key.NameEscape})
			if !ok {
				return
			}
			if event, ok := event.(key.Event); ok && event.State == key.Press {
				tbCtx.CancelMoveCue()
				return
			}
		}
	}

	for {
		event, ok := gtx.Event(
			key.Filter{Name: "N", Required: key.ModShortcut},
			key.Filter{Name: "S", Required: key.ModShortcut},
			key.Filter{Name: "S", Required: key.ModShortcut | key.ModShift},
		)
		if !ok {
			break
		}
		keyEvent, ok := event.(key.Event)
		if !ok {
			continue
		}
		if dispatchDocumentShortcut(topBar, keyEvent) {
			return
		}
	}

	// Let a focused top-bar button retain its normal Enter/Space behavior.
	if topBar.HasKeyboardFocus(gtx) || playbackSidebar.HasKeyboardFocus(gtx) {
		return
	}

	for {
		event, ok := gtx.Event(
			key.Filter{Name: key.NameUpArrow},
			key.Filter{Name: key.NameDownArrow},
			key.Filter{Name: key.NamePageUp},
			key.Filter{Name: key.NamePageDown},
			key.Filter{Name: key.NameHome},
			key.Filter{Name: key.NameEnd},
			key.Filter{Name: key.NameReturn},
			key.Filter{Name: key.NameEnter},
			key.Filter{Name: key.NameF2},
			key.Filter{Name: key.NameSpace, Optional: key.ModShift},
			key.Filter{Name: key.NameDeleteForward},
			key.Filter{Name: "C", Required: key.ModShortcut},
			key.Filter{Name: "V", Required: key.ModShortcut},
			key.Filter{Name: "D", Required: key.ModShortcut},
			key.Filter{Name: "M", Required: key.ModShortcut},
			key.Filter{Name: "E", Required: key.ModShortcut},
		)
		if !ok {
			return
		}
		keyEvent, ok := event.(key.Event)
		if !ok || keyEvent.State != key.Press {
			continue
		}

		switch keyEvent.Name {
		case key.NameUpArrow:
			manager.MoveSelection(-1)
		case key.NameDownArrow:
			manager.MoveSelection(1)
		case key.NamePageUp:
			manager.MoveSelection(-10)
		case key.NamePageDown:
			manager.MoveSelection(10)
		case key.NameHome:
			manager.SelectCue(0)
		case key.NameEnd:
			manager.SelectCue(manager.CueCount() - 1)
		case key.NameReturn, key.NameEnter, key.NameF2:
			tbCtx.EditSelectedCue(manager)
		case key.NameSpace:
			var err error
			if keyEvent.Modifiers.Contain(key.ModShift) {
				err = playbackEngine.PlaySelectedOverride()
				if err == nil {
					a.UI.OperatorPanel.DismissBlocker()
				}
			} else {
				err = playbackEngine.PlaySelected()
			}
			if err != nil {
				log.Printf("play cue: %v", err)
			}
		case key.NameDeleteForward:
			tbCtx.RequestDeleteCue(manager)
		case "C":
			tbCtx.CopySelectedCue(manager)
		case "V":
			tbCtx.PasteCueBeforeSelected(manager)
		case "D":
			tbCtx.DuplicateSelectedCue(manager)
		case "M":
			tbCtx.StartMoveCue(manager)
		case "E":
			tbCtx.EditSelectedCue(manager)
		}
	}
}

func dispatchDocumentShortcut(bar *ui.TopBar, event key.Event) bool {
	if bar == nil || event.State != key.Press || !event.Modifiers.Contain(key.ModShortcut) {
		return false
	}
	switch event.Name {
	case "N":
		bar.RequestNew()
	case "S":
		if event.Modifiers.Contain(key.ModShift) {
			bar.RequestSaveAs()
		} else {
			bar.RequestSave()
		}
	default:
		return false
	}
	return true
}

func layoutFocusWarning(th *material.Theme, gtx layout.Context) layout.Dimensions {
	return warningBar(gtx, unit.Dp(88), func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			label := material.H3(th, "** WARNING ** NO FOCUS **")
			label.Color = palette.White
			return label.Layout(gtx)
		})
	})
}

func layoutAudioWarning(th *material.Theme, gtx layout.Context, warning string, settingsButton *widget.Clickable) layout.Dimensions {
	return warningBar(gtx, unit.Dp(118), func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(24), Right: unit.Dp(24), Top: unit.Dp(14), Bottom: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.H4(th, "** WARNING ** AUDIO OUTPUT UNAVAILABLE **")
							label.Color = palette.White
							return label.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.Body1(th, warning)
							label.Color = palette.White
							return label.Layout(gtx)
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					button := material.Button(th, settingsButton, "Open audio settings")
					button.Background = palette.SurfaceSunken
					return button.Layout(gtx)
				}),
			)
		})
	})
}

func layoutVideoOutputWarning(th *material.Theme, gtx layout.Context, warning string) layout.Dimensions {
	return warningBar(gtx, unit.Dp(92), func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(24), Right: unit.Dp(24), Top: unit.Dp(12), Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := material.H5(th, "** WARNING ** VIDEO DISPLAY MISSING **")
					label.Color = palette.White
					return label.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := material.Body1(th, warning)
					label.Color = palette.White
					return label.Layout(gtx)
				}),
			)
		})
	})
}

func layoutSafetyWarning(th *material.Theme, gtx layout.Context, warning string, resume *widget.Clickable) layout.Dimensions {
	return warningBar(gtx, unit.Dp(118), func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(24), Right: unit.Dp(24), Top: unit.Dp(14), Bottom: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.H4(th, "** PLAYBACK STOPPED AFTER SYSTEM INTERRUPTION **")
							label.Color = palette.White
							return label.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.Body1(th, warning)
							label.Color = palette.White
							return label.Layout(gtx)
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					button := material.Button(th, resume, "Acknowledge and re-arm GO")
					button.Background = palette.SurfaceSunken
					return button.Layout(gtx)
				}),
			)
		})
	})
}

func warningBar(gtx layout.Context, requestedHeight unit.Dp, content layout.Widget) layout.Dimensions {
	size := gtx.Constraints.Max
	height := min(gtx.Dp(requestedHeight), size.Y)
	size.Y = height
	paint.FillShape(gtx.Ops, palette.Danger, clip.Rect{Max: size}.Op())
	gtx.Constraints.Min = size
	gtx.Constraints.Max = size
	return content(gtx)
}

func layoutWarnings(th *material.Theme, gtx layout.Context, windowFocused bool, audioWarning, videoWarning, safetyWarning string, settingsButton, safetyResume *widget.Clickable) layout.Dimensions {
	return layout.S.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = 0
		children := make([]layout.FlexChild, 0, 2)
		if safetyWarning != "" {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutSafetyWarning(th, gtx, safetyWarning, safetyResume)
			}))
		}
		if !windowFocused {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutFocusWarning(th, gtx)
			}))
		}
		if audioWarning != "" {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutAudioWarning(th, gtx, audioWarning, settingsButton)
			}))
		}
		if videoWarning != "" {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutVideoOutputWarning(th, gtx, videoWarning)
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

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
	audioWarningSettings := &a.UI.AudioWarningSettings
	var ops op.Ops
	windowFocused := true
	th := newTheme()
	expl := explorer.NewExplorer(window)
	uiActions := make(chan func(), 16)
	var documentMu sync.RWMutex
	var saveMu sync.Mutex
	currentShowPath := a.RecoveredPath
	lastSavedDigest := showDigest(manager.ShowSnapshot())
	if a.Recovered {
		lastSavedDigest = [sha256.Size]byte{}
		topBar.SetStatus("Recovered unsaved show edits · save to confirm recovery")
	}
	var suppressJournal bool
	var documentGuard ui.DocumentGuard
	var closeInterceptor windowCloseInterceptor
	closeRequests := make(chan struct{}, 1)
	var lastAudioOperatorWarning, lastVideoOperatorWarning string
	var safetyResume widget.Clickable
	lastFrameAt := time.Now()
	power := startPowerKeeper()
	defer power.Close()
	preflightService := newPreflightService()
	defer preflightService.Close()
	playbackEngine.SetPreflightGate(func() error { return preflightService.Gate(manager.ShowSnapshot()) })
	tbCtx.LoadWaveform = func(source string, completed func([]float32, int, int64, error)) {
		go func() {
			wave, err := media.ExtractWaveform(settingsStore.Snapshot().FFmpegPath, source)
			if err != nil {
				operatorEvents.Add(operatorlog.Recoverable, "FFmpeg waveform", err.Error(), show.CueID{}, "")
			}
			uiActions <- func() { completed(wave.Samples, wave.SampleRate, wave.DurationMs, err) }
			window.Invalidate()
		}()
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
				operatorEvents.Add(operatorlog.ShowStopping, "Edit recovery", err.Error(), show.CueID{}, "")
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
		go func() {
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
				uiActions <- func() { topBar.SetStatus("Could not add file: " + err.Error()) }
				window.Invalidate()
				return
			}

			uiActions <- func() {
				selected(entry.Source)
				if duplicate {
					topBar.SetStatus("Duplicate detected · using existing " + entry.Name)
				} else {
					topBar.SetStatus("Added " + entry.Name)
				}
			}
			window.Invalidate()
		}()
	}

	loadShow := func() {
		go func() {
			file, err := expl.ChooseFile(".cusus")
			if err != nil {
				if !errors.Is(err, explorer.ErrUserDecline) {
					operatorEvents.Add(operatorlog.Recoverable, "Open show", err.Error(), show.CueID{}, "")
					uiActions <- func() { topBar.SetStatus("Open failed: " + err.Error()) }
					window.Invalidate()
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
				uiActions <- func() { topBar.SetStatus("Open failed: " + err.Error()) }
				window.Invalidate()
				return
			}
			manifest, files, err := project.Load(tmp.Name())
			if err != nil {
				operatorEvents.Add(operatorlog.Recoverable, "Open show", err.Error(), show.CueID{}, "")
				uiActions <- func() { topBar.SetStatus("Open failed: " + err.Error()) }
				window.Invalidate()
				return
			}
			uiActions <- func() {
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
				topBar.SetStatus("Loaded " + documentName(loadedPath) + " · recovery journal on")
			}
			window.Invalidate()
		}()
	}

	var saveAsShow func(func(bool))
	var saveShow func(func(bool))

	saveAsShow = func(done func(bool)) {
		complete := func(success bool) {
			if done != nil {
				uiActions <- func() { done(success) }
				window.Invalidate()
			}
		}
		go func() {
			file, err := expl.CreateFile("show.cusus")
			if err != nil {
				if !errors.Is(err, explorer.ErrUserDecline) {
					operatorEvents.Add(operatorlog.Recoverable, "Save show", err.Error(), show.CueID{}, "")
					uiActions <- func() { topBar.SetStatus("Save failed: " + err.Error()) }
					window.Invalidate()
				}
				complete(false)
				return
			}
			path := explorerPath(file)
			if closeErr := file.Close(); closeErr != nil {
				operatorEvents.Add(operatorlog.Recoverable, "Save show", closeErr.Error(), show.CueID{}, "")
				complete(false)
				return
			}
			if strings.TrimSpace(path) == "" {
				operatorEvents.Add(operatorlog.Recoverable, "Save show", "file picker did not return a filesystem path", show.CueID{}, "")
				complete(false)
				return
			}
			uiActions <- func() { topBar.SetStatus("Saving and optimizing bundled media…") }
			window.Invalidate()
			snapshot := manager.ShowSnapshot()
			saveMu.Lock()
			manifest, err := saveShowAtPath(path, snapshot, settingsStore.Snapshot().FFmpegPath)
			saveMu.Unlock()
			if err != nil {
				operatorEvents.Add(operatorlog.Recoverable, "FFmpeg / save show", err.Error(), show.CueID{}, "")
				uiActions <- func() { topBar.SetStatus("Save failed: " + err.Error()) }
				window.Invalidate()
				complete(false)
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
			uiActions <- func() {
				topBar.SetStatus("Saved " + documentName(path) + " · recovery journal on · " + formatFileCount(len(manifest.Assets)))
			}
			window.Invalidate()
			complete(true)
		}()
	}

	saveShow = func(done func(bool)) {
		documentMu.RLock()
		path := currentShowPath
		documentMu.RUnlock()
		if path == "" {
			saveAsShow(done)
			return
		}
		go func() {
			snapshot := manager.ShowSnapshot()
			uiActions <- func() { topBar.SetStatus("Saving " + documentName(path) + "…") }
			window.Invalidate()
			saveMu.Lock()
			manifest, err := saveShowAtPath(path, snapshot, settingsStore.Snapshot().FFmpegPath)
			saveMu.Unlock()
			if err != nil {
				operatorEvents.Add(operatorlog.Recoverable, "Save show", err.Error(), show.CueID{}, "")
				uiActions <- func() { topBar.SetStatus("Save failed: " + err.Error()) }
				window.Invalidate()
				if done != nil {
					uiActions <- func() { done(false) }
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
			uiActions <- func() {
				topBar.SetStatus("Saved " + documentName(path) + " · recovery journal on · " + formatFileCount(len(manifest.Assets)))
			}
			window.Invalidate()
			if done != nil {
				uiActions <- func() { done(true) }
			}
		}()
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
		topBar.SetStatus("New untitled show · recovery journal on")
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
					operatorEvents.Add(operatorlog.ShowStopping, "Edit recovery", err.Error(), show.CueID{}, "")
				}
			}
			playbackEngine.Close()
			mediaManager.Close()
			return e.Err

		case app.ConfigEvent:
			windowFocused = e.Config.Focused
			window.Invalidate()

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
				playbackEngine.AcknowledgeSafetyLatch()
				topBar.SetStatus("Playback re-armed after operator acknowledgement · press GO when ready")
			}
			audioWarning := mediaManager.AudioDeviceWarning()
			videoWarning := videoRoutingWarning(mediaManager)
			if audioWarning != "" && audioWarning != lastAudioOperatorWarning {
				operatorEvents.Add(operatorlog.ShowStopping, "Audio output", audioWarning, show.CueID{}, "")
			}
			if videoWarning != "" && videoWarning != lastVideoOperatorWarning {
				operatorEvents.Add(operatorlog.ShowStopping, "Video output", videoWarning, show.CueID{}, "")
			}
			lastAudioOperatorWarning, lastVideoOperatorWarning = audioWarning, videoWarning
			preflight := preflightService.Request(manager.ShowSnapshot(), settingsStore.Snapshot(), audioWarning, videoWarning, playbackEngine.RemoteHealth(), playbackEngine.CueProblems)
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

func formatFileCount(count int) string {
	if count == 1 {
		return "1 media file"
	}
	return fmt.Sprintf("%d media files", count)
}

func explorerPath(file any) string {
	var source string
	switch file := file.(type) {
	case *explorer.File:
		source = file.URI()
	case *os.File:
		source = file.Name()
	}
	path, err := project.LocalPath(source)
	if err != nil {
		return ""
	}
	return path
}

func documentName(path string) string {
	if strings.TrimSpace(path) == "" {
		return "show.cusus"
	}
	return filepath.Base(path)
}

func showDigest(current show.Show) [sha256.Size]byte {
	raw, _ := json.Marshal(current)
	return sha256.Sum256(raw)
}

func saveShowAtPath(path string, current show.Show, ffmpegPath string) (project.Manifest, error) {
	if strings.TrimSpace(path) == "" {
		return project.Manifest{}, errors.New("show has no file path; use Save As")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return project.Manifest{}, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".cusus-save-*")
	if err != nil {
		return project.Manifest{}, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	manifest, err := project.Save(tmp, current, ffmpegPath)
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return project.Manifest{}, err
	}
	if err := os.Rename(tmpPath, path); err == nil {
		return manifest, nil
	}

	// Windows does not consistently replace an existing file with Rename.
	// Keep the old document as a short-lived backup until the new one lands.
	backup := path + ".autosave-backup"
	_ = os.Remove(backup)
	if err := os.Rename(path, backup); err != nil {
		return project.Manifest{}, fmt.Errorf("replace show file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Rename(backup, path)
		return project.Manifest{}, fmt.Errorf("replace show file: %w", err)
	}
	_ = os.Remove(backup)
	return manifest, nil
}

func buildPreflight(cues []show.Cue, settings config.Settings, audioWarning, videoWarning string) []operatorlog.PreflightCheck {
	return buildPreflightWithProblems(cues, settings, audioWarning, videoWarning, func(cue show.Cue) []show.CueProblem {
		return show.CueProblemsWithContext(cue, cues, show.WarningContext{Settings: settings})
	})
}

func buildPreflightWithProblems(cues []show.Cue, settings config.Settings, audioWarning, videoWarning string, problemsForCue func(show.Cue) []show.CueProblem) []operatorlog.PreflightCheck {
	checks := make([]operatorlog.PreflightCheck, 0)
	if len(cues) == 0 {
		checks = append(checks, operatorlog.PreflightCheck{Severity: operatorlog.Warning, Source: "Show", Message: "The show contains no cues"})
	}
	needsFFmpeg, hasRemote := false, false
	for _, cue := range cues {
		needsFFmpeg = needsFFmpeg || cue.Type == show.CueTypeSound || cue.Type == show.CueTypeVideo
		hasRemote = hasRemote || cue.Type == show.CueTypeRemote
		for _, problem := range problemsForCue(cue) {
			if problem.Severity == show.ProblemState {
				if problem.Code != "media.check.pending" && problem.Code != "media.check.not-run" {
					continue
				}
				checks = append(checks, operatorlog.PreflightCheck{
					Severity: operatorlog.ShowStopping, Code: problem.Code, Source: "Media readiness",
					Message: problem.Message, Consequence: problem.Consequence, Fix: problem.Fix, Field: problem.Field,
					CueID: cue.ID, CueNumber: cue.CueNumber, Fingerprint: show.ProblemFingerprint(cue, problem, settings),
				})
				continue
			}
			checks = append(checks, operatorlog.PreflightCheck{
				Severity: preflightProblemSeverity(problem.Severity), Code: problem.Code, Source: "Cue configuration",
				Message: problem.Message, Consequence: problem.Consequence, Fix: problem.Fix, Field: problem.Field,
				CueID: cue.ID, CueNumber: cue.CueNumber, Fingerprint: show.ProblemFingerprint(cue, problem, settings),
			})
		}
	}
	if needsFFmpeg {
		if _, err := findExecutable(settings.FFmpegPath); err != nil {
			checks = append(checks, operatorlog.PreflightCheck{Severity: operatorlog.ShowStopping, Source: "FFmpeg", Message: err.Error()})
		}
		probe := ffprobeExecutable(settings.FFmpegPath)
		if _, err := findExecutable(probe); err != nil {
			checks = append(checks, operatorlog.PreflightCheck{Severity: operatorlog.ShowStopping, Source: "FFprobe", Message: err.Error()})
		}
	}
	if hasRemote && len(settings.RemoteTargets) == 0 {
		checks = append(checks, operatorlog.PreflightCheck{Severity: operatorlog.ShowStopping, Source: "Network / remote control", Message: "Remote cues exist but no remote targets are configured"})
	}
	if audioWarning != "" {
		checks = append(checks, operatorlog.PreflightCheck{Severity: operatorlog.ShowStopping, Source: "Audio output", Message: audioWarning})
	}
	if videoWarning != "" {
		checks = append(checks, operatorlog.PreflightCheck{Severity: operatorlog.ShowStopping, Source: "Video output", Message: videoWarning})
	}
	return checks
}

func preflightProblemSeverity(severity show.ProblemSeverity) operatorlog.Severity {
	if severity == show.ProblemBlocker {
		return operatorlog.ShowStopping
	}
	return operatorlog.Warning
}

func findExecutable(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("executable path is empty")
	}
	resolved, err := exec.LookPath(path)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	return resolved, nil
}

func ffprobeExecutable(ffmpegPath string) string {
	if filepath.IsAbs(ffmpegPath) {
		return filepath.Join(filepath.Dir(ffmpegPath), "ffprobe"+filepath.Ext(ffmpegPath))
	}
	return "ffprobe"
}
