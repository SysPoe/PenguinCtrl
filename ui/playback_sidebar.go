package ui

import (
	"fmt"
	"image/color"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/playback"
	"github.com/syspoe/cusus/show"
)

const playbackSidebarWidth = unit.Dp(280)

// TODO(macro): PlaybackSidebar drives engine.ControlMedia from Layout and uses raw widget.Float instead of the ui/input slider kit, while re-deriving duration/position math already present on the cue list. Treat transport as a small controller (commands out, instance snapshot in) and share runtime formatting/normalization with the list presentation layer.
type PlaybackSidebar struct {
	goButton       widget.Clickable
	stopAllButton  widget.Clickable
	fadeAllButton  widget.Clickable
	pauseButton    widget.Clickable
	stopButton     widget.Clickable
	fadeOutButton  widget.Clickable
	restartButton  widget.Clickable
	endJumpButton  widget.Clickable
	positionSlider widget.Float
	volumeSlider   widget.Float
	instanceID     string
}

func (s *PlaybackSidebar) HasKeyboardFocus(gtx layout.Context) bool {
	return gtx.Focused(&s.goButton) || gtx.Focused(&s.stopAllButton) ||
		gtx.Focused(&s.fadeAllButton) || gtx.Focused(&s.pauseButton) ||
		gtx.Focused(&s.stopButton) || gtx.Focused(&s.fadeOutButton) ||
		gtx.Focused(&s.restartButton) || gtx.Focused(&s.endJumpButton) ||
		gtx.Focused(&s.positionSlider) || gtx.Focused(&s.volumeSlider)
}

func (s *PlaybackSidebar) Layout(th *material.Theme, gtx layout.Context, manager *show.ShowManager, engine *playback.Engine) layout.Dimensions {
	width := min(gtx.Constraints.Max.X, gtx.Dp(playbackSidebarWidth))
	gtx.Constraints.Min.X = width
	gtx.Constraints.Max.X = width

	selected, _, hasSelection := manager.SelectedCueCopy()
	instance, hasInstance := selectedInstance(engine, selected.ID, hasSelection)
	selectedActive := hasSelection && engine.CueActive(selected.ID)
	if hasInstance && instance.ID != s.instanceID {
		s.instanceID = instance.ID
		s.positionSlider.Value = normalizedPosition(instance)
		s.volumeSlider.Value = normalizedVolume(instance.LevelDB)
	} else if !hasInstance {
		s.instanceID = ""
	}

	if hasSelection {
		for {
			click, ok := s.goButton.Update(gtx)
			if !ok {
				break
			}
			if click.Modifiers.Contain(key.ModShift) {
				_ = engine.PlaySelectedOverride()
			} else {
				_ = engine.PlaySelected()
			}
		}
	}
	if s.stopAllButton.Clicked(gtx) {
		engine.StopAll()
	}
	if s.fadeAllButton.Clicked(gtx) {
		engine.FadeAll()
	}
	if hasInstance {
		target := show.MediaTarget{Kind: show.MediaTargetInstance, InstanceID: instance.ID}
		if s.pauseButton.Clicked(gtx) {
			action := show.MediaControlPause
			if instance.Paused {
				action = show.MediaControlResume
			}
			_ = engine.ControlMedia(target, action, nil, nil, 0)
		}
		if s.stopButton.Clicked(gtx) {
			_ = engine.ControlMedia(target, show.MediaControlStop, nil, nil, 0)
		}
		if s.fadeOutButton.Clicked(gtx) {
			_ = engine.FadeInstance(instance.ID)
		}
		if s.restartButton.Clicked(gtx) {
			position := instance.ClipStartMs
			_ = engine.ControlMedia(target, show.MediaControlSeek, nil, &position, 0)
			_ = engine.ControlMedia(target, show.MediaControlResume, nil, nil, 0)
		}
		if s.endJumpButton.Clicked(gtx) {
			engine.EndInstance(instance.ID)
		}
	}

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
							sidebarButton(th, &s.pauseButton, pauseLabel(instance, hasInstance), hasInstance, palette.SurfaceSunken, palette.Text, 1),
							sidebarButton(th, &s.stopButton, "Stop", hasInstance, palette.SurfaceSunken, palette.Warning, 1),
							sidebarButton(th, &s.fadeOutButton, "Fade Out", hasInstance, palette.SurfaceSunken, palette.Primary, 1),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
								sidebarButton(th, &s.restartButton, "Restart", hasInstance, palette.SurfaceSunken, palette.Text, 1),
								sidebarButton(th, &s.endJumpButton, "End Jump", hasInstance, palette.SurfaceSunken, palette.Text, 1),
							)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.layoutPosition(th, gtx, engine, instance, hasInstance)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.layoutVolume(th, gtx, engine, instance, hasInstance)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return sidebarActionButton(th, gtx, &s.goButton, goButtonLabel(selectedActive), hasSelection, palette.SurfaceSunken, palette.Text)
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

func (s *PlaybackSidebar) layoutPosition(th *material.Theme, gtx layout.Context, engine *playback.Engine, instance playback.Instance, enabled bool) layout.Dimensions {
	duration := instance.DurationMs
	if duration <= 0 && instance.ClipEndMs > instance.ClipStartMs {
		duration = instance.ClipEndMs - instance.ClipStartMs
	}
	enabled = enabled && duration > 0
	if enabled && !s.positionSlider.Dragging() {
		s.positionSlider.Value = normalizedPosition(instance)
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
				dims := material.Slider(th, &s.positionSlider).Layout(gtx)
				if enabled && s.positionSlider.Value != before {
					position := instance.ClipStartMs + int64(float64(duration)*float64(s.positionSlider.Value))
					_ = engine.ControlMedia(show.MediaTarget{Kind: show.MediaTargetInstance, InstanceID: instance.ID}, show.MediaControlSeek, nil, &position, 0)
				}
				return dims
			}),
		)
	})
}

func (s *PlaybackSidebar) layoutVolume(th *material.Theme, gtx layout.Context, engine *playback.Engine, instance playback.Instance, enabled bool) layout.Dimensions {
	enabled = enabled && (instance.MediaType == "audio" || instance.MediaType == "video")
	if enabled && !s.volumeSlider.Dragging() {
		s.volumeSlider.Value = normalizedVolume(instance.LevelDB)
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
				dims := material.Slider(th, &s.volumeSlider).Layout(gtx)
				if enabled && s.volumeSlider.Value != before {
					// TODO(micro): -80..+12 dB range (92 span) is magic; name volumeMinDB/volumeRangeDB consts and share with normalizedVolume.
					level := -80 + float64(s.volumeSlider.Value)*92
					_ = engine.ControlMedia(show.MediaTarget{Kind: show.MediaTargetInstance, InstanceID: instance.ID}, show.MediaControlSetVolume, &level, nil, 0)
				}
				return dims
			}),
		)
	})
}

func selectedInstance(engine *playback.Engine, cueID show.CueID, selected bool) (playback.Instance, bool) {
	if !selected || engine == nil {
		return playback.Instance{}, false
	}
	var latest playback.Instance
	found := false
	for _, instance := range engine.ActiveInstances() {
		if instance.CueID == cueID && (!found || instance.StartedAt.After(latest.StartedAt)) {
			latest, found = instance, true
		}
	}
	return latest, found
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
	duration := instance.DurationMs
	if duration <= 0 && instance.ClipEndMs > instance.ClipStartMs {
		duration = instance.ClipEndMs - instance.ClipStartMs
	}
	if duration <= 0 {
		return 0
	}
	return min(float32(1), max(float32(0), float32(instance.PositionMs-instance.ClipStartMs)/float32(duration)))
}

func normalizedVolume(levelDB float64) float32 {
	// TODO(micro): +80/92 volume mapping duplicates layoutVolume seek math; share volumeMinDB/volumeRangeDB consts.
	return min(float32(1), max(float32(0), float32((levelDB+80)/92)))
}

// TODO(micro): Remove weight while every caller passes 1; the unused variability makes call sites noisier.
func sidebarButton(th *material.Theme, clickable *widget.Clickable, label string, enabled bool, bg, fg color.NRGBA, weight float32) layout.FlexChild {
	return layout.Flexed(weight, func(gtx layout.Context) layout.Dimensions {
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
