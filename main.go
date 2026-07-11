package main

import (
	"errors"
	"image"
	"log"
	"os"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"gioui.org/x/explorer"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/media"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/playback"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/ui"
)

var topBar ui.TopBar
var playbackSidebar ui.PlaybackSidebar
var tbCtx ui.TBContext
var manager *show.ShowManager = show.NewShowManager()
var settingsStore *config.Store
var playbackEngine *playback.Engine
var mediaManager *media.Manager
var settingsPage *ui.SettingsPage
var showSettings bool

func main() {
	var err error
	settingsStore, err = config.Open("")
	if err != nil {
		log.Fatal(err)
	}
	playbackEngine = playback.NewEngine(manager, settingsStore)
	playbackEngine.SetDurationProbe(func(source string) (int64, error) {
		return media.ProbeDurationMs(settingsStore.Snapshot().FFmpegPath, source)
	})
	playbackEngine.Start()
	mediaManager = media.NewManager(playbackEngine, settingsStore)
	mediaManager.SyncOutputs(playbackEngine.OutputIDs())
	settingsPage = ui.NewSettingsPage(settingsStore)
	settingsPage.SetOnSaved(func() {
		playbackEngine.RefreshDurations()
		mediaManager.SyncOutputs(playbackEngine.OutputIDs())
	})
	settingsPage.SetOnReopenOutputs(func() {
		mediaManager.EnsureOutputs(playbackEngine.OutputIDs())
	})
	tbCtx = ui.TBContext{
		TopBar:        &topBar,
		TogglePreview: playbackEngine.TogglePreview,
		StopPreview:   playbackEngine.StopPreview,
	}
	go func() {
		window := new(app.Window)
		window.Option(
			app.Title("CuSus"),
			app.Size(unit.Dp(1300), unit.Dp(720)),
			app.MinSize(unit.Dp(1300), unit.Dp(720)),
		)
		if err := run(window); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
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

func handleCueListShortcuts(gtx layout.Context) {
	if showSettings || tbCtx.CueEditorOpen() {
		return
	}
	if tbCtx.DeleteConfirmationOpen() {
		tbCtx.HandleDeleteConfirmationKeys(gtx, manager)
		return
	}

	if topBar.AddCueMenuOpen() || topBar.ActionMenuOpen() {
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
			key.Filter{Name: key.NameSpace},
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
			if err := playbackEngine.PlaySelected(); err != nil {
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

func layoutFocusWarning(th *material.Theme, gtx layout.Context) layout.Dimensions {
	size := gtx.Constraints.Max
	height := gtx.Dp(unit.Dp(88))
	if height > size.Y {
		height = size.Y
	}
	top := size.Y - height

	paint.FillShape(
		gtx.Ops,
		palette.Danger,
		clip.Rect{Min: image.Pt(0, top), Max: size}.Op(),
	)

	offset := op.Offset(image.Pt(0, top)).Push(gtx.Ops)
	defer offset.Pop()
	gtx.Constraints.Min = image.Pt(size.X, height)
	gtx.Constraints.Max = gtx.Constraints.Min
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		label := material.H3(th, "** WARNING ** NO FOCUS **")
		label.Color = palette.White
		return label.Layout(gtx)
	})
}

func run(window *app.Window) error {
	var ops op.Ops
	windowFocused := true
	th := newTheme()
	expl := explorer.NewExplorer(window)
	uiActions := make(chan func(), 16)
	tbCtx.LoadWaveform = func(source string, completed func([]float32, int, int64, error)) {
		go func() {
			wave, err := media.ExtractWaveform(settingsStore.Snapshot().FFmpegPath, source)
			uiActions <- func() { completed(wave.Samples, wave.SampleRate, wave.DurationMs, err) }
			window.Invalidate()
		}()
	}
	playbackEngine.SetOnChange(func() {
		window.Invalidate()
		mediaManager.SyncOutputs(playbackEngine.OutputIDs())
	})
	manager.SetOnChange(func() {
		window.Invalidate()
		playbackEngine.RefreshDurations()
		mediaManager.SyncOutputs(playbackEngine.OutputIDs())
	})

	tbCtx.PickFile = func(extensions []string, selected func(path string)) {
		go func() {
			file, err := expl.ChooseFile(extensions...)
			if err != nil {
				if !errors.Is(err, explorer.ErrUserDecline) {
					log.Printf("pick file: %v", err)
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

			uiActions <- func() { selected(path) }
			window.Invalidate()
		}()
	}

	for {
		e := window.Event()
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

		switch e := e.(type) {
		case app.DestroyEvent:
			playbackEngine.Close()
			return e.Err

		case app.ConfigEvent:
			windowFocused = e.Config.Focused
			window.Invalidate()

		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			handleCueListShortcuts(gtx)

			paint.Fill(gtx.Ops, th.Bg)

			layout.Stack{}.Layout(gtx,
				// Main application layout
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{
						Axis: layout.Vertical,
					}.Layout(gtx,
						// Top Bar
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return topBar.Layout(th, gtx, manager.HasSelectedCue(), showSettings)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							if showSettings {
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
										th, gtx, manager, playbackEngine,
										func() { tbCtx.EditSelectedCue(manager) },
										tbCtx.MoveCueActive(),
										func(index int) { tbCtx.MoveSelectedCueBefore(manager, index) },
										func() { tbCtx.MoveSelectedCueToEnd(manager) },
									)
								}),
							)
						}),
					)
				}),
				// Top Bar Submenus
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					if showSettings {
						return layout.Dimensions{}
					}
					return tbCtx.Layout(th, gtx, manager)
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					if windowFocused {
						return layout.Dimensions{}
					}
					return layoutFocusWarning(th, gtx)
				}),
			)
			if topBar.TakePageRequest() {
				showSettings = !showSettings
				window.Invalidate()
			}
			e.Frame(gtx.Ops)
		}
	}
}
