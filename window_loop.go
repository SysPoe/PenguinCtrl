package main

import (
	"context"
	"errors"
	"image"
	"log"
	"strings"
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
	"github.com/syspoe/cusus/project"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/ui"
)

const (
	windowTaskConcurrency = 4
	windowShutdownTimeout = 3 * time.Second
	frameRefreshInterval  = time.Second
	compactLayoutWidth    = unit.Dp(850)
)

func (s *windowSession) run() error {
	a, window := s.application, s.window
	topBar := &a.UI.TopBar
	playbackSidebar := &a.UI.PlaybackSidebar
	tbCtx := &a.UI.TBContext
	manager := a.Document.Show
	settingsStore := a.Document.Settings
	playbackEngine := a.Playback.Engine
	mediaManager := a.Playback.Media
	settingsPage := a.UI.SettingsPage
	projectLibrary := a.UI.ProjectLibrary
	operatorEvents := a.Operator.Log
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
	tasks := taskgroup.New(context.Background(), windowTaskConcurrency, a.CrashReporter)
	defer func() {
		if err := tasks.Close(windowShutdownTimeout); err != nil {
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
	document := newDocumentSession(a.Recovery.Path, manager.ShowSnapshot(), a.Recovery.Recovered)
	if a.Recovery.Recovered {
		operatorPanel.SetStatus("Recovered unsaved show edits · save to confirm recovery")
		operatorEvents.Diagnostic("Edit recovery", "Recovered unsaved show edits", map[string]any{"documentPath": document.pathSnapshot()})
	}
	var documentGuard ui.DocumentGuard
	var closeInterceptor windowCloseInterceptor
	closeRequests := make(chan struct{}, 1)
	var safetyResume widget.Clickable
	healthCollectors := newHealthCollectors(
		playbackEngine, mediaManager, a.Playback.Timecode, a.Playback.Redundancy, settingsStore.Snapshot,
		func() (string, bool) {
			path, dirty, _ := document.status(manager.ShowSnapshot())
			return path, dirty
		},
	)
	healthMonitor := health.NewMonitor(func() []health.Component { return health.CollectAll(healthCollectors...) }, readinessRefreshInterval)
	defer healthMonitor.Close()
	defer a.Playback.Timecode.Close()
	defer a.Playback.Redundancy.Close()
	power := startPowerKeeper(func(err error) {
		operatorEvents.Add(operatorlog.ShowStopping, "Power management", "Windows could not keep the show computer awake: "+err.Error(), show.CueID{}, "")
	})
	defer power.Close()
	preflightService, err := newPreflightService()
	if err != nil {
		return err
	}
	defer preflightService.Close()
	playbackEngine.SetPreflightGate(func(cue show.Cue) error { return preflightService.Gate(manager.ShowSnapshot(), cue) })
	playbackEngine.SetAuthorityGate(a.Playback.Redundancy.Gate)
	playbackEngine.SetRemoteAuthorityExecutor(a.Playback.Redundancy.WithAuthority)
	frameController := newOperatorFrameController(operatorFrameConfig{
		playback: playbackEngine, media: mediaManager,
		timecode: a.Playback.Timecode, redundancy: a.Playback.Redundancy,
		settings: settingsStore, show: manager, library: projectLibrary,
		events: operatorEvents, panel: operatorPanel, topBar: topBar,
		audioWarningSettings: audioWarningSettings, safetyResume: &safetyResume,
		tasks: tasks, postUI: postUI, health: healthMonitor, preflight: preflightService,
		showAudioSettings: func() {
			a.UI.ShowSettings = true
			settingsPage.ShowAudioDevices()
		},
	})
	cacheMaintainer := project.StartCacheMaintainer(
		func() bool {
			return len(playbackEngine.ActiveInstances()) > 0 || len(playbackEngine.ActiveExecutions()) > 0
		},
		func() []string {
			current, settings := manager.ShowSnapshot(), settingsStore.Snapshot()
			paths := make([]string, 0, len(current.Cues))
			for _, cue := range current.Cues {
				paths = append(paths, project.ResolvedMediaSources(cue, settings)...)
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
		path, dirty, suppressed := document.status(snapshot)
		if !suppressed && dirty && a.Document.Journal != nil {
			if err := a.Document.Journal.RecordDirty(snapshot, path); err != nil {
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
			defer func() { _ = file.Close() }()

			path := explorerPath(file)
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

	documents := newDocumentController(documentControllerConfig{
		explorer: expl, tasks: tasks, postUI: postUI, manager: manager,
		playback: playbackEngine, library: projectLibrary, session: document,
		journal: a.Document.Journal, settings: settingsStore,
		events: operatorEvents, panel: operatorPanel,
	})
	loadShow, saveShow, saveAsShow, performNew := documents.Load, documents.Save, documents.SaveAs, documents.New
	performDocumentAction := func(action ui.DocumentAction) {
		switch action {
		case ui.DocumentActionNew:
			performNew()
		case ui.DocumentActionOpen:
			loadShow()
		case ui.DocumentActionClose:
			if err := closeInterceptor.AllowAndClose(); err != nil {
				operatorEvents.Add(operatorlog.Recoverable, "Close protection", err.Error(), show.CueID{}, "")
			}
		}
	}
	layoutCueList := func(gtx layout.Context) layout.Dimensions {
		return ui.Main(
			th, gtx, &a.UI.CueList, manager, playbackEngine, operatorEvents,
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
			_, dirty, _ := document.status(snapshot)
			if documentGuard.Request(ui.DocumentActionClose, dirty) {
				if err := closeInterceptor.AllowAndClose(); err != nil {
					operatorEvents.Add(operatorlog.Recoverable, "Close protection", err.Error(), show.CueID{}, "")
				}
			}
		default:
		}

		switch e := e.(type) {
		case app.DestroyEvent:
			snapshot := manager.ShowSnapshot()
			path, dirty, _ := document.status(snapshot)
			if dirty && a.Document.Journal != nil {
				if err := a.Document.Journal.RecordDirty(snapshot, path); err != nil {
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
			playbackEngine.Close()
			a.Playback.Remote.Close()
			if err := mediaManager.Close(); err != nil {
				operatorEvents.Diagnostic("Media shutdown", err.Error(), nil)
			}
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
					tasks.Go("operator-window-placement", func(context.Context) {
						if err := applyOperatorPlacement(handle, placement); err != nil {
							operatorEvents.Add(operatorlog.Recoverable, "Operator window", err.Error(), show.CueID{}, "")
						}
					})
				}
			}

		case app.FrameEvent:
			now := time.Now()
			gtx := app.NewContext(&ops, e)
			gtx.Execute(op.InvalidateCmd{At: time.Now().Add(frameRefreshInterval)})
			preflight := frameController.Update(gtx, now)
			audioWarning, videoWarning := frameController.Warnings()
			a.handleCueListShortcuts(gtx)
			showState := manager.ShowSnapshot()
			_, dirty, _ := document.status(showState)
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
							if gtx.Constraints.Max.X < gtx.Dp(compactLayoutWidth) {
								return layoutCueList(gtx)
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
									return layoutCueList(gtx)
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
					return ui.LayoutWarnings(th, gtx, windowFocused, audioWarning, videoWarning, playbackEngine.SafetyLatchReason(), audioWarningSettings, &safetyResume)
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
