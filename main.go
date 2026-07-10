package main

import (
	"errors"
	"image/color"
	"log"
	"os"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"gioui.org/x/explorer"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/media"
	"github.com/syspoe/cusus/playback"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/ui"
)

var topBar ui.TopBar
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
	tbCtx = ui.TBContext{
		TopBar: &topBar,
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

	darkBg := color.NRGBA{R: 0x12, G: 0x12, B: 0x12, A: 0xFF}         // Dark gray surface
	darkBgContrast := color.NRGBA{R: 0x30, G: 0x1E, B: 0x30, A: 0xFF} // Slightly lighter gray for contrast
	darkFg := color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xDE}         // Off-white for high contrast
	darkFgContrast := color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF} // Pure white for maximum contrast

	// Apply colors to the Theme Palette
	th.Palette.Bg = darkBg
	th.Palette.ContrastBg = darkBgContrast
	th.Palette.Fg = darkFg
	th.Palette.ContrastFg = darkFgContrast

	return th
}

func handleCueListShortcuts(gtx layout.Context) {
	if showSettings || tbCtx.CueEditorOpen() {
		return
	}

	if topBar.AddCueMenuOpen() {
		for {
			event, ok := gtx.Event(key.Filter{Name: key.NameEscape})
			if !ok {
				return
			}
			if event, ok := event.(key.Event); ok && event.State == key.Press {
				topBar.CloseAddCueMenu()
				return
			}
		}
	}

	// Let a focused top-bar button retain its normal Enter/Space behavior.
	if topBar.HasKeyboardFocus(gtx) {
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
		}
	}
}

func run(window *app.Window) error {
	var ops op.Ops
	th := newTheme()
	expl := explorer.NewExplorer(window)
	uiActions := make(chan func(), 16)
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
							return ui.Main(th, gtx, manager, playbackEngine, func() { tbCtx.EditSelectedCue(manager) })
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
			)
			if topBar.TakePageRequest() {
				showSettings = !showSettings
				window.Invalidate()
			}
			if topBar.TakeGoRequest() {
				if err := playbackEngine.PlaySelected(); err != nil {
					log.Printf("play cue: %v", err)
				}
			}
			if topBar.TakeStopRequest() {
				playbackEngine.StopAll()
			}
			e.Frame(gtx.Ops)
		}
	}
}
