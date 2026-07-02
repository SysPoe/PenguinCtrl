package main

import (
	"errors"
	"image/color"
	"log"
	"os"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"gioui.org/x/explorer"

	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/ui"
)

var topBar ui.TopBar
var tbCtx ui.TBContext
var manager *show.ShowManager = show.NewShowManager()

func main() {
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
	th.Face = font.Typeface("Times New Roman, Noto Sans Sinhala, sans-serif")

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

func run(window *app.Window) error {
	var ops op.Ops
	th := newTheme()
	expl := explorer.NewExplorer(window)
	uiActions := make(chan func(), 16)

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
			return e.Err

		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)

			paint.Fill(gtx.Ops, th.Bg)

			layout.Stack{}.Layout(gtx,
				// Main application layout
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{
						Axis: layout.Vertical,
					}.Layout(gtx,
						// Top Bar
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return topBar.Layout(th, gtx)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return ui.Main(th, gtx, manager)
						}),
					)
				}),
				// Top Bar Submenus
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return tbCtx.Layout(th, gtx, manager)
				}),
			)
			e.Frame(gtx.Ops)
		}
	}
}
