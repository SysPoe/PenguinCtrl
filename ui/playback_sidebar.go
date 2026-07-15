package ui

import (
	"fmt"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/playback"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/ui/input"
)

const playbackSidebarWidth = unit.Dp(280)

type PlaybackSidebar struct {
	goButton       widget.Clickable
	stopAllButton  widget.Clickable
	fadeAllButton  widget.Clickable
	pauseButton    widget.Clickable
	stopButton     widget.Clickable
	fadeOutButton  widget.Clickable
	restartButton  widget.Clickable
	endJumpButton  widget.Clickable
	positionSlider *input.Slider
	volumeSlider   *input.Slider
	instanceID     string
}

const (
	volumeMinDB = -80.0
	volumeMaxDB = 12.0
)

func (s *PlaybackSidebar) ensureSliders() {
	if s.positionSlider == nil {
		s.positionSlider = input.NewSlider("Position", 0, 1, 0)
	}
	if s.volumeSlider == nil {
		s.volumeSlider = input.NewSlider("Volume", volumeMinDB, volumeMaxDB, 0)
	}
}

func (s *PlaybackSidebar) HasKeyboardFocus(gtx layout.Context) bool {
	s.ensureSliders()
	return gtx.Focused(&s.goButton) || gtx.Focused(&s.stopAllButton) ||
		gtx.Focused(&s.fadeAllButton) || gtx.Focused(&s.pauseButton) ||
		gtx.Focused(&s.stopButton) || gtx.Focused(&s.fadeOutButton) ||
		gtx.Focused(&s.restartButton) || gtx.Focused(&s.endJumpButton) ||
		s.positionSlider.Focused(gtx) || s.volumeSlider.Focused(gtx)
}

func (s *PlaybackSidebar) Layout(th *material.Theme, gtx layout.Context, manager *show.ShowManager, engine *playback.Engine) layout.Dimensions {
	width := min(gtx.Constraints.Max.X, gtx.Dp(playbackSidebarWidth))
	gtx.Constraints.Min.X = width
	gtx.Constraints.Max.X = width

	s.ensureSliders()
	controller := newPlaybackSidebarController(engine)
	snapshot := controller.snapshot(manager)
	if snapshot.hasInstance && snapshot.instance.ID != s.instanceID {
		s.instanceID = snapshot.instance.ID
		s.positionSlider.Value = float64(normalizedPosition(snapshot.instance))
		s.volumeSlider.Value = snapshot.instance.LevelDB
	} else if !snapshot.hasInstance {
		s.instanceID = ""
	}
	controller.update(gtx, s, snapshot)
	selected, hasSelection := snapshot.selected, snapshot.hasSelection
	instance, hasInstance := snapshot.instance, snapshot.hasInstance

	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, palette.Surface, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						label := material.H6(th, selectedCueTitle(selected, hasSelection))
						label.MaxLines = 2
						return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, label.Layout)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
							sidebarButton(th, &s.pauseButton, pauseLabel(instance, hasInstance), hasInstance, palette.SurfaceSunken, palette.Text),
							sidebarButton(th, &s.stopButton, "Stop", hasInstance, palette.SurfaceSunken, palette.Warning),
							sidebarButton(th, &s.fadeOutButton, "Fade Out", hasInstance, palette.SurfaceSunken, palette.Primary),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
								sidebarButton(th, &s.restartButton, "Restart", hasInstance, palette.SurfaceSunken, palette.Text),
								sidebarButton(th, &s.endJumpButton, "End Jump", hasInstance, palette.SurfaceSunken, palette.Text),
							)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.layoutPosition(th, gtx, controller, instance, hasInstance)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.layoutVolume(th, gtx, controller, instance, hasInstance)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return sidebarActionButton(th, gtx, &s.goButton, goButtonLabel(snapshot.selectedActive), hasSelection, palette.SurfaceSunken, palette.Text)
						})
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return sidebarActionButton(th, gtx, &s.stopAllButton, "STOP ALL", true, palette.SurfaceSunken, palette.Warning)
						})
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return sidebarActionButton(th, gtx, &s.fadeAllButton, "FADE ALL", true, palette.SurfaceSunken, palette.Primary)
						})
					}),
				)
			})
		},
	)
}

func goButtonLabel(active bool) string {
	if active {
		return "RESTART"
	}
	return "GO"
}

func (s *PlaybackSidebar) layoutPosition(th *material.Theme, gtx layout.Context, controller playbackSidebarController, instance playback.Instance, enabled bool) layout.Dimensions {
	duration := playbackDuration(instance)
	enabled = enabled && duration > 0
	if enabled && !s.positionSlider.Dragging() {
		s.positionSlider.Value = float64(normalizedPosition(instance))
	}
	before := s.positionSlider.Value
	return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := material.Body2(th, fmt.Sprintf("Position  %s / %s", formatRuntimeTimeValue(max(int64(0), instance.PositionMs-instance.ClipStartMs)), formatRuntimeTime(duration)))
				return label.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !enabled {
					gtx = gtx.Disabled()
				}
				dims := s.positionSlider.Layout(th, gtx)
				if enabled && s.positionSlider.Value != before {
					position := instance.ClipStartMs + int64(float64(duration)*s.positionSlider.Value)
					controller.seek(instance, position)
				}
				return dims
			}),
		)
	})
}

func (s *PlaybackSidebar) layoutVolume(th *material.Theme, gtx layout.Context, controller playbackSidebarController, instance playback.Instance, enabled bool) layout.Dimensions {
	enabled = enabled && (instance.MediaType == playback.MediaTypeAudio || instance.MediaType == playback.MediaTypeVideo)
	if enabled && !s.volumeSlider.Dragging() {
		s.volumeSlider.Value = instance.LevelDB
	}
	before := s.volumeSlider.Value
	return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				value := "-"
				if enabled {
					value = fmt.Sprintf("%.1f dB", instance.LevelDB)
				}
				return material.Body2(th, "Volume  "+value).Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !enabled {
					gtx = gtx.Disabled()
				}
				dims := s.volumeSlider.Layout(th, gtx)
				if enabled && s.volumeSlider.Value != before {
					controller.setVolume(instance, s.volumeSlider.Value)
				}
				return dims
			}),
		)
	})
}

func selectedCueTitle(cue show.Cue, selected bool) string {
	if !selected {
		return "No cue selected"
	}
	if cue.Description == "" {
		return "Cue " + cue.CueNumber
	}
	return fmt.Sprintf("Cue %s - %s", cue.CueNumber, cue.Description)
}

func pauseLabel(instance playback.Instance, active bool) string {
	if active && instance.Paused {
		return "Resume"
	}
	return "Pause"
}

func normalizedPosition(instance playback.Instance) float32 {
	duration := playbackDuration(instance)
	if duration <= 0 {
		return 0
	}
	return min(float32(1), max(float32(0), float32(instance.PositionMs-instance.ClipStartMs)/float32(duration)))
}

func sidebarButton(th *material.Theme, clickable *widget.Clickable, label string, enabled bool, bg, fg color.NRGBA) layout.FlexChild {
	return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		if !enabled {
			gtx = gtx.Disabled()
			fg = palette.Disabled
		}
		return layout.Inset{Right: unit.Dp(3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			button := material.Button(th, clickable, label)
			button.Background = bg
			button.Color = fg
			button.TextSize = unit.Sp(12)
			return button.Layout(gtx)
		})
	})
}

func sidebarActionButton(th *material.Theme, gtx layout.Context, clickable *widget.Clickable, label string, enabled bool, bg, fg color.NRGBA) layout.Dimensions {
	if !enabled {
		gtx = gtx.Disabled()
		fg = palette.Disabled
	}
	gtx.Constraints.Min = gtx.Constraints.Max
	button := material.Button(th, clickable, label)
	button.Background = bg
	button.Color = fg
	button.TextSize = unit.Sp(24)
	return button.Layout(gtx)
}
