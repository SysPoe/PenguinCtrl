package media

import (
	"image/color"
	"log"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	oto "github.com/hajimehoshi/oto/v2"
	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/playback"
)

type Manager struct {
	engine   *playback.Engine
	settings *config.Store
	mu       sync.Mutex
	windows  map[string]*outputWindow
	audio    *oto.Context
	ready    <-chan struct{}
}

func NewManager(engine *playback.Engine, settings *config.Store) *Manager {
	context, ready, err := oto.NewContext(48000, 2, oto.FormatSignedInt16LE)
	if err != nil {
		log.Printf("initialize audio output: %v", err)
	}
	return &Manager{engine: engine, settings: settings, windows: map[string]*outputWindow{}, audio: context, ready: ready}
}

func (m *Manager) EnsureOutputs(outputIDs []string) {
	for _, outputID := range outputIDs {
		m.ensureOutput(outputID)
	}
}

func (m *Manager) SyncOutputs(outputIDs []string) {
	desired := make(map[string]struct{}, len(outputIDs))
	for _, outputID := range outputIDs {
		desired[outputID] = struct{}{}
		m.ensureOutput(outputID)
	}
	m.mu.Lock()
	var stale []*outputWindow
	for outputID, output := range m.windows {
		if _, keep := desired[outputID]; !keep {
			stale = append(stale, output)
		}
	}
	m.mu.Unlock()
	for _, output := range stale {
		if output.window != nil {
			output.window.Perform(system.ActionClose)
		}
	}
}

func (m *Manager) ensureOutput(outputID string) {
	if outputID == "" {
		return
	}
	m.mu.Lock()
	if _, exists := m.windows[outputID]; exists {
		m.mu.Unlock()
		return
	}
	output := &outputWindow{id: outputID, manager: m, players: map[string]*Player{}}
	m.windows[outputID] = output
	m.mu.Unlock()
	go output.run()
}

func (m *Manager) removed(outputID string) {
	m.mu.Lock()
	delete(m.windows, outputID)
	m.mu.Unlock()
}

type outputWindow struct {
	id              string
	manager         *Manager
	window          *app.Window
	players         map[string]*Player
	clickable       widget.Clickable
	fullscreen      bool
	blackout        bool
	test            bool
	identify        bool
	identifyMessage string
	reopening       bool
	transition      *outputTransition
}

type outputTransition struct {
	event   playback.Event
	stage   string
	started time.Time
}

func (o *outputWindow) run() {
	defer func() {
		o.manager.removed(o.id)
		if o.reopening {
			o.manager.ensureOutput(o.id)
		}
	}()
	log.Printf("opening media output %q", o.id)
	o.window = new(app.Window)
	o.window.Option(app.Title(o.id), app.Size(unit.Dp(960), unit.Dp(540)), app.MinSize(unit.Dp(320), unit.Dp(180)))
	events, unsubscribe := o.manager.engine.Subscribe(o.id)
	defer unsubscribe()
	pending := make(chan playback.Event, 64)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case event := <-events:
				select {
				case pending <- event:
					o.window.Invalidate()
				case <-done:
					return
				}
			case <-done:
				return
			}
		}
	}()

	var ops op.Ops
	for {
		event := o.window.Event()
		switch event := event.(type) {
		case app.DestroyEvent:
			log.Printf("media output %q closed: %v", o.id, event.Err)
			for _, player := range o.players {
				player.Close(false)
			}
			return
		case app.FrameEvent:
			gtx := app.NewContext(&ops, event)
		pendingLoop:
			for {
				select {
				case mediaEvent := <-pending:
					o.handleEvent(mediaEvent)
				default:
					break pendingLoop
				}
			}
			for {
				click, ok := o.clickable.Update(gtx)
				if !ok {
					break
				}
				if click.NumClicks == 2 {
					o.fullscreen = !o.fullscreen
					if o.fullscreen {
						o.window.Option(app.Fullscreen.Option())
					} else {
						o.window.Option(app.Windowed.Option())
					}
				}
			}
			paint.Fill(gtx.Ops, color.NRGBA{A: 0xFF})
			o.clickable.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return o.layout(gtx)
			})
			event.Frame(gtx.Ops)
		}
	}
}

func (o *outputWindow) layout(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min = gtx.Constraints.Max
	o.advanceTransition()
	if o.transition != nil {
		gtx.Execute(op.InvalidateCmd{At: time.Now().Add(time.Second / 60)})
	}
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return o.layoutContent(gtx)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			opacity := o.transitionOpacity()
			if opacity <= 0 {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			}
			black := color.NRGBA{A: uint8(min(float32(1), opacity) * 255)}
			paint.FillShape(gtx.Ops, black, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
	)
}

func (o *outputWindow) layoutContent(gtx layout.Context) layout.Dimensions {
	if o.blackout {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	if o.test {
		return layoutTestPattern(gtx)
	}
	var visible *Player
	for _, player := range o.players {
		if player.MediaType() == "video" || player.MediaType() == "image" {
			if visible == nil || player.StartedAt().After(visible.StartedAt()) {
				visible = player
			}
		}
	}
	if visible != nil {
		return visible.Layout(gtx)
	}
	if o.identify {
		th := material.NewTheme()
		text := o.identifyMessage
		if text == "" {
			text = o.id
		}
		label := material.H3(th, text)
		label.Color = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
		return layout.Center.Layout(gtx, label.Layout)
	}
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func (o *outputWindow) handleEvent(event playback.Event) {
	switch event.Action {
	case "sync":
		for _, instance := range event.Instances {
			instance := instance
			o.start(&instance)
		}
	case "play":
		o.start(event.Instance)
	case "remove":
		for _, id := range event.InstanceIDs {
			if player := o.players[id]; player != nil {
				player.Close(false)
				delete(o.players, id)
			}
		}
	case "control":
		for _, id := range event.InstanceIDs {
			if player := o.players[id]; player != nil {
				player.Control(event)
			}
		}
	case "output":
		o.handleOutputControl(event)
	}
}

func (o *outputWindow) start(instance *playback.Instance) {
	if instance == nil || o.players[instance.ID] != nil {
		return
	}
	player := NewPlayer(
		*instance,
		o.manager.settings,
		o.manager.audio,
		o.manager.ready,
		o.window,
		func(report string) { o.manager.engine.HandleOutputReport(instance.ID, report) },
		func(durationMs int64) { o.manager.engine.HandleOutputDuration(instance.ID, durationMs) },
	)
	o.players[instance.ID] = player
	go func() {
		if err := player.Start(); err != nil {
			log.Printf("play %s: %v", instance.Source, err)
			o.manager.engine.HandleOutputReport(instance.ID, "stopped")
		}
	}()
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
		o.fullscreen = true
		o.window.Option(app.Fullscreen.Option())
	case "exit-fullscreen":
		o.fullscreen = false
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
		barColor := barColor
		children[i] = layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, barColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		})
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}
