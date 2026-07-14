package media

import (
	"context"
	"errors"
	"image"
	"image/color"
	"log"
	"sort"
	"time"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/playback"
)

func resolveDisplayForGeometry(id string, displays []VideoDisplay) (VideoDisplay, bool) {
	if id != "" {
		for _, display := range displays {
			if display.ID == id {
				return display, true
			}
		}
	}
	for _, display := range displays {
		if display.Primary {
			return display, id == ""
		}
	}
	return displays[0], id == ""
}

func (o *outputWindow) layout(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min = gtx.Constraints.Max
	o.advanceTransition()
	if o.transition != nil {
		gtx.Execute(op.InvalidateCmd{At: time.Now().Add(time.Second / 60)})
	}
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return o.layoutCanvas(gtx)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			opacity := o.transitionOpacity()
			if opacity <= 0 {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			}
			black := palette.WithAlpha(palette.Black, uint8(min(float32(1), opacity)*255))
			paint.FillShape(gtx.Ops, black, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
	)
}

func (o *outputWindow) layoutCanvas(gtx layout.Context) layout.Dimensions {
	route := o.route()
	canvas := gtx.Constraints.Max
	if route.ResolutionWidth > 0 && route.ResolutionHeight > 0 && canvas.X > 0 && canvas.Y > 0 {
		height := canvas.X * route.ResolutionHeight / route.ResolutionWidth
		if height <= canvas.Y {
			canvas.Y = height
		} else {
			canvas.X = canvas.Y * route.ResolutionWidth / route.ResolutionHeight
		}
	}
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min, gtx.Constraints.Max = canvas, canvas
		defer clip.Rect{Max: canvas}.Push(gtx.Ops).Pop()
		return layout.Stack{}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions { return o.layoutContent(gtx, route) }),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions { return o.layoutGuides(gtx, route) }),
		)
	})
}

func (o *outputWindow) layoutContent(gtx layout.Context, route config.VideoOutput) layout.Dimensions {
	if o.blackout {
		o.setVisibleDecoders(nil)
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	if o.test {
		o.setVisibleDecoders(nil)
		return layoutTestPattern(gtx)
	}
	visible := make([]*Player, 0, len(o.players))
	for _, player := range o.players {
		if player.MediaType() == "video" || player.MediaType() == "image" {
			visible = append(visible, player)
		}
	}
	sort.SliceStable(visible, func(i, j int) bool { return playerLayerLess(visible[i], visible[j]) })
	visible = playersForLayers(visible, route.Layers)
	o.setVisibleDecoders(visible)
	if len(visible) > 0 {
		children := make([]layout.StackChild, 0, len(visible))
		for _, player := range visible {
			// TODO(micro): Remove this obsolete loop-variable copy; Go 1.22+ closures capture the per-iteration player.
			player := player
			children = append(children, layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				return player.LayoutScaled(gtx, route.Scaling)
			}))
		}
		return layout.Stack{}.Layout(gtx, children...)
	}
	if route.IdleBehavior == "hold" && o.heldFrame != nil {
		return layoutFrame(gtx, o.heldFrame, route.Scaling)
	}
	if o.identify {
		th := material.NewTheme()
		text := o.identifyMessage
		if text == "" {
			text = o.id
		}
		label := material.H3(th, text)
		label.Color = palette.White
		return layout.Center.Layout(gtx, label.Layout)
	}
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func playersForLayers(players []*Player, layers int) []*Player {
	layers = max(1, layers)
	if len(players) <= layers {
		return players
	}
	first := len(players) - layers
	selected := append([]*Player(nil), players[first:]...)
	included := make(map[*Player]struct{}, len(selected)+1)
	for _, player := range selected {
		included[player] = struct{}{}
	}
	// For a single-layer output, continue drawing the last successfully
	// presented layer beneath an incoming player until its first frame arrives.
	if layers == 1 && !selected[0].HasPresented() {
		for i := first - 1; i >= 0; i-- {
			if players[i].HasPresented() {
				selected = append(selected, players[i])
				included[players[i]] = struct{}{}
				break
			}
		}
	}
	// A replaced layer is a temporary crossfade participant, not an additional
	// persistent output layer.
	for _, player := range players[:first] {
		if _, exists := included[player]; !exists && player.VisualFadeOutActive() {
			selected = append(selected, player)
		}
	}
	sort.SliceStable(selected, func(i, j int) bool { return playerLayerLess(selected[i], selected[j]) })
	return selected
}

func playerLayerLess(first, second *Player) bool {
	if first.instance.LayerOrder != second.instance.LayerOrder {
		return first.instance.LayerOrder < second.instance.LayerOrder
	}
	if !first.StartedAt().Equal(second.StartedAt()) {
		return first.StartedAt().Before(second.StartedAt())
	}
	return first.instance.ID < second.instance.ID
}

func (o *outputWindow) setVisibleDecoders(visible []*Player) {
	shown := make(map[*Player]struct{}, len(visible))
	for _, player := range visible {
		shown[player] = struct{}{}
	}
	for _, player := range o.players {
		_, ok := shown[player]
		player.SetDecodeVisible(ok)
	}
}

func (o *outputWindow) layoutGuides(gtx layout.Context, route config.VideoOutput) layout.Dimensions {
	size := gtx.Constraints.Max
	if route.TestGrid {
		line := max(1, min(size.X, size.Y)/360)
		grid := color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0x58}
		for i := 1; i < 10; i++ {
			x := size.X * i / 10
			y := size.Y * i / 10
			paint.FillShape(gtx.Ops, grid, clip.Rect{Min: image.Pt(x, 0), Max: image.Pt(x+line, size.Y)}.Op())
			paint.FillShape(gtx.Ops, grid, clip.Rect{Min: image.Pt(0, y), Max: image.Pt(size.X, y+line)}.Op())
		}
	}
	if route.SafeAreaPercent > 0 {
		insetX, insetY := size.X*route.SafeAreaPercent/100, size.Y*route.SafeAreaPercent/100
		line := max(1, min(size.X, size.Y)/240)
		guide := color.NRGBA{R: 0xFF, G: 0xD5, B: 0x4A, A: 0xD0}
		paintRectOutline(gtx, image.Rect(insetX, insetY, size.X-insetX, size.Y-insetY), line, guide)
	}
	o.routeMu.Lock()
	displayMissing := o.displayMissing
	o.routeMu.Unlock()
	if displayMissing {
		th := material.NewTheme()
		label := material.Body1(th, "ASSIGNED DISPLAY MISSING · TEMPORARY PRIMARY OUTPUT")
		label.Color = palette.White
		return layout.N.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(18)}.Layout(gtx, label.Layout)
		})
	}
	return layout.Dimensions{Size: size}
}

func paintRectOutline(gtx layout.Context, rect image.Rectangle, width int, fill color.NRGBA) {
	paint.FillShape(gtx.Ops, fill, clip.Rect{Min: rect.Min, Max: image.Pt(rect.Max.X, rect.Min.Y+width)}.Op())
	paint.FillShape(gtx.Ops, fill, clip.Rect{Min: image.Pt(rect.Min.X, rect.Max.Y-width), Max: rect.Max}.Op())
	paint.FillShape(gtx.Ops, fill, clip.Rect{Min: rect.Min, Max: image.Pt(rect.Min.X+width, rect.Max.Y)}.Op())
	paint.FillShape(gtx.Ops, fill, clip.Rect{Min: image.Pt(rect.Max.X-width, rect.Min.Y), Max: rect.Max}.Op())
}

func layoutFrame(gtx layout.Context, frame image.Image, scaling string) layout.Dimensions {
	player := &Player{frame: frame}
	return player.LayoutScaled(gtx, scaling)
}

func (o *outputWindow) handleEvent(event playback.Event) {
	if event.Action == "resync" {
		events, sequence := o.manager.engine.OutputSnapshot(o.id)
		for _, snapshotEvent := range events {
			o.applyEvent(snapshotEvent)
		}
		o.lastSequence = sequence
		return
	}
	if event.Sequence > 0 && event.Sequence <= o.lastSequence {
		return
	}
	o.applyEvent(event)
	if event.Sequence > o.lastSequence {
		o.lastSequence = event.Sequence
	}
}

func (o *outputWindow) applyEvent(event playback.Event) {
	switch event.Action {
	case "sync":
		o.reconcile(event.Instances)
	case "play":
		o.start(event.Instance)
	case "remove":
		for _, id := range event.InstanceIDs {
			if player := o.players[id]; player != nil {
				if o.route().IdleBehavior == "hold" {
					o.heldFrame = player.Frame()
				}
				player.Close(false)
				delete(o.players, id)
			}
		}
	case "control":
		if event.Control == "stop-all" {
			o.heldFrame = nil
			for id, player := range o.players {
				player.Close(false)
				delete(o.players, id)
			}
			return
		}
		for _, id := range event.InstanceIDs {
			if player := o.players[id]; player != nil {
				player.Control(event)
			}
		}
	case "output":
		o.handleOutputControl(event)
	}
}

func (o *outputWindow) reconcile(instances []playback.Instance) {
	desired := make(map[string]playback.Instance, len(instances))
	for _, instance := range instances {
		desired[instance.ID] = instance
	}
	for id, player := range o.players {
		if _, keep := desired[id]; keep {
			continue
		}
		player.Close(false)
		delete(o.players, id)
	}
	for _, instance := range instances {
		if o.players[instance.ID] != nil {
			continue
		}
		// TODO(micro): Remove this obsolete loop-variable copy; the module requires Go 1.26.
		instance := instance
		o.start(&instance)
	}
}

func (o *outputWindow) start(instance *playback.Instance) {
	if instance == nil || o.players[instance.ID] != nil {
		return
	}
	backend := o.manager.playbackBackend()
	if backend == nil {
		o.manager.engine.HandleOutputError(instance.ID, errors.New("media backend is unavailable"))
		return
	}
	player := NewPlayerWithBackend(
		*instance,
		o.manager.settings,
		backend,
		o.window,
		func(report string) { o.manager.engine.HandleOutputReport(instance.ID, report) },
		func(durationMs int64) { o.manager.engine.HandleOutputDuration(instance.ID, durationMs) },
		func(err error) { o.manager.engine.HandleOutputError(instance.ID, err) },
	)
	o.players[instance.ID] = player
	player.goOwned(func(context.Context) {
		if err := player.Start(); err != nil {
			log.Printf("play %s: %v", instance.Source, err)
			o.manager.engine.HandleOutputError(instance.ID, err)
		}
	})
}

func (o *outputWindow) handleOutputControl(event playback.Event) {
	if event.Control == "fullscreen" || event.Control == "exit-fullscreen" || event.Control == "reopen" {
		o.applyOutputControl(event)
		return
	}
	if event.FadeOutMs > 0 {
		o.transition = &outputTransition{event: event, stage: "out", started: time.Now()}
		o.window.Invalidate()
		return
	}
	o.applyOutputControl(event)
	if event.FadeInMs > 0 && event.Control != "blackout" {
		o.transition = &outputTransition{event: event, stage: "in", started: time.Now()}
		o.window.Invalidate()
	}
}

func (o *outputWindow) applyOutputControl(event playback.Event) {
	switch event.Control {
	case "blackout":
		o.blackout = true
	case "clear":
		o.blackout, o.test, o.identify, o.identifyMessage = false, false, false, ""
		o.heldFrame = nil
		for id, player := range o.players {
			player.Close(true)
			delete(o.players, id)
		}
	case "test-pattern":
		o.test, o.blackout, o.identify = true, false, false
	case "identify":
		o.identify, o.blackout, o.test = true, false, false
		o.identifyMessage = event.Message
	case "reopen":
		o.reopening = true
		o.window.Perform(system.ActionClose)
	case "fullscreen":
		o.routeMu.Lock()
		o.fullscreen = true
		o.routeMu.Unlock()
		o.window.Option(app.Fullscreen.Option())
	case "exit-fullscreen":
		o.routeMu.Lock()
		o.fullscreen = false
		o.routeMu.Unlock()
		o.window.Option(app.Windowed.Option())
	}
}

func (o *outputWindow) advanceTransition() {
	transition := o.transition
	if transition == nil {
		return
	}
	durationMs := transition.event.FadeInMs
	if transition.stage == "out" {
		durationMs = transition.event.FadeOutMs
	}
	if durationMs > 0 && time.Since(transition.started) < time.Duration(durationMs)*time.Millisecond {
		return
	}
	if transition.stage == "out" {
		o.applyOutputControl(transition.event)
		if transition.event.Control != "blackout" && transition.event.Control != "reopen" && transition.event.FadeInMs > 0 {
			transition.stage = "in"
			transition.started = time.Now()
			return
		}
	}
	o.transition = nil
}

func (o *outputWindow) transitionOpacity() float32 {
	transition := o.transition
	if transition == nil {
		return 0
	}
	durationMs := transition.event.FadeInMs
	if transition.stage == "out" {
		durationMs = transition.event.FadeOutMs
	}
	if durationMs <= 0 {
		if transition.stage == "out" {
			return 1
		}
		return 0
	}
	progress := min(float32(1), float32(time.Since(transition.started))/float32(time.Duration(durationMs)*time.Millisecond))
	if transition.stage == "in" {
		return 1 - progress
	}
	return progress
}

var testPatternColors = []color.NRGBA{
	{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}, {R: 0xFF, G: 0xFF, A: 0xFF},
	{G: 0xFF, B: 0xFF, A: 0xFF}, {G: 0xFF, A: 0xFF},
	{R: 0xFF, B: 0xFF, A: 0xFF}, {R: 0xFF, A: 0xFF},
	{B: 0xFF, A: 0xFF}, {A: 0xFF},
}

func layoutTestPattern(gtx layout.Context) layout.Dimensions {
	children := make([]layout.FlexChild, len(testPatternColors))
	for i, barColor := range testPatternColors {
		// TODO(micro): Remove this obsolete loop-variable copy; barColor is already scoped to this iteration.
		barColor := barColor
		children[i] = layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, barColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		})
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}
