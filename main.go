package main

import (
	"image/color"
	"log"
	"os"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/widget/material"

	"github.com/SysPoe/CuSus/show"
	"github.com/SysPoe/CuSus/ui"
)

var topBar ui.TopBar
var tbCtx ui.TbContext
var manager *show.ShowManager = show.NewShowManager()

func main() {
	tbCtx = ui.TbContext{
		TopBar: &topBar,
	}
	go func() {
		window := new(app.Window)
		window.Option(app.Title("CuSus"))
		if err := run(window); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

func NewTheme() *material.Theme {
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
	th := NewTheme()

	for {
		switch e := window.Event().(type) {
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
