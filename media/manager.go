package media

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"log"
	"sort"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/playback"
)

type Manager struct {
	engine            *playback.Engine
	settings          *config.Store
	mu                sync.Mutex
	windows           map[string]*outputWindow
	desired           map[string]struct{}
	closed            bool
	ctx               context.Context
	cancel            context.CancelFunc
	workers           sync.WaitGroup
	audio             *AudioSystem
	decoder           *FFmpegBackend
	audioStatusMu     sync.Mutex
	lastAudioCheck    time.Time
	audioDeviceStatus string
	audioDevices      []AudioDevice
	audioDevicesErr   error
	audioRefresh      chan struct{}
	displaysMu        sync.RWMutex
	displays          []VideoDisplay
	displaysErr       error
	displayRefresh    chan struct{}
	displaySignature  string
	displayStatusMu   sync.Mutex
	lastDisplayCheck  time.Time
	videoOutputStatus string
}

func NewManager(engine *playback.Engine, settings *config.Store) *Manager {
	audioSystem, err := NewAudioSystem(settings)
	if err != nil {
		log.Printf("initialize audio output: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	manager := &Manager{engine: engine, settings: settings, windows: map[string]*outputWindow{}, desired: map[string]struct{}{}, audio: audioSystem, ctx: ctx, cancel: cancel, audioRefresh: make(chan struct{}, 1), displayRefresh: make(chan struct{}, 1), audioDeviceStatus: "Checking audio output devices…"}
	manager.decoder = NewFFmpegBackend(settings, audioSystem)
	manager.refreshDisplays(true)
	manager.workers.Add(2)
	go func() { defer manager.workers.Done(); manager.monitorDisplays() }()
	go func() { defer manager.workers.Done(); manager.monitorAudioDevices() }()
	return manager
}

func (m *Manager) Prewarm(instances []playback.Instance) {
	requests := make([]PlaybackRequest, 0, len(instances))
	for _, instance := range instances {
		requests = append(requests, PlaybackRequest{
			Instance: instance,
			Position: time.Duration(max(int64(0), instance.ClipStartMs)) * time.Millisecond,
		})
	}
	m.decoder.Prewarm(requests)
}

func (m *Manager) AudioDevices() ([]AudioDevice, error) {
	m.audioStatusMu.Lock()
	defer m.audioStatusMu.Unlock()
	return append([]AudioDevice(nil), m.audioDevices...), m.audioDevicesErr
}

func (m *Manager) AudioMixerMetrics() []AudioMixerMetrics {
	if m.audio == nil {
		return nil
	}
	return m.audio.Metrics()
}

// AudioDeviceWarning returns a cached warning for selected devices that are no
// longer present. Empty device IDs intentionally follow Windows' default route
// and therefore do not depend on one particular endpoint remaining connected.
func (m *Manager) AudioDeviceWarning() string {
	m.audioStatusMu.Lock()
	defer m.audioStatusMu.Unlock()
	return m.audioDeviceStatus
}

func (m *Manager) RefreshAudioDeviceStatus() {
	select {
	case m.audioRefresh <- struct{}{}:
	default:
	}
}

func (m *Manager) monitorAudioDevices() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		m.refreshAudioDevices()
		select {
		case <-m.ctx.Done():
			return
		case <-m.audioRefresh:
		case <-ticker.C:
		}
	}
}

func (m *Manager) refreshAudioDevices() {
	var devices []AudioDevice
	var err error
	if m.audio == nil {
		err = fmt.Errorf("audio output is unavailable")
	} else {
		devices, err = m.audio.Devices()
	}
	status := audioDeviceWarning(m.settings.Snapshot(), devices, err)
	if status == "" {
		for _, metrics := range m.AudioMixerMetrics() {
			if metrics.Failed {
				status = "An audio endpoint could not be recovered. Active cues on that route are offline."
				break
			}
			if metrics.Recovering {
				status = "An audio endpoint stopped unexpectedly. CuSus is reconnecting with bounded retry."
				break
			}
		}
	}
	m.audioStatusMu.Lock()
	m.audioDevices, m.audioDevicesErr, m.audioDeviceStatus, m.lastAudioCheck = devices, err, status, time.Now()
	m.audioStatusMu.Unlock()
}

func audioDeviceWarning(settings config.Settings, devices []AudioDevice, err error) string {
	if err != nil {
		return "Audio device detection failed: " + err.Error()
	}
	if len(devices) == 0 {
		return "No Windows audio output device is available. Playback and preview audio are offline."
	}
	available := make(map[string]struct{}, len(devices))
	for _, device := range devices {
		available[device.ID] = struct{}{}
	}
	_, playbackAvailable := available[settings.PlaybackAudioDevice]
	_, previewAvailable := available[settings.PreviewAudioDevice]
	playbackMissing := settings.PlaybackAudioDevice != "" && !playbackAvailable
	previewMissing := settings.PreviewAudioDevice != "" && !previewAvailable
	switch {
	case playbackMissing && previewMissing && settings.PlaybackAudioDevice == settings.PreviewAudioDevice:
		return "The selected playback and preview audio device is disconnected."
	case playbackMissing && previewMissing:
		return "The selected playback and preview audio devices are disconnected."
	case playbackMissing:
		return "The selected playback audio device is disconnected."
	case previewMissing:
		return "The selected preview audio device is disconnected."
	default:
		return ""
	}
}

func (m *Manager) EnsureOutputs(outputIDs []string) {
	outputIDs = m.outputIDsWithConfiguredStages(outputIDs)
	m.mu.Lock()
	for _, outputID := range outputIDs {
		m.desired[outputID] = struct{}{}
	}
	m.mu.Unlock()
	for _, outputID := range outputIDs {
		m.ensureOutput(outputID)
	}
}

func (m *Manager) SyncOutputs(outputIDs []string) {
	outputIDs = m.outputIDsWithConfiguredStages(outputIDs)
	desired := make(map[string]struct{}, len(outputIDs))
	for _, outputID := range outputIDs {
		desired[outputID] = struct{}{}
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.desired = desired
	var stale []*outputWindow
	for outputID, output := range m.windows {
		if _, keep := desired[outputID]; !keep {
			stale = append(stale, output)
		}
	}
	m.mu.Unlock()
	for _, outputID := range outputIDs {
		m.ensureOutput(outputID)
	}
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
	if m.closed {
		m.mu.Unlock()
		return
	}
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

func (m *Manager) shouldRecoverOutput(outputID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return false
	}
	_, desired := m.desired[outputID]
	return desired
}

func (m *Manager) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	m.cancel()
	m.desired = map[string]struct{}{}
	windows := make([]*outputWindow, 0, len(m.windows))
	for _, output := range m.windows {
		windows = append(windows, output)
	}
	m.mu.Unlock()
	m.workers.Wait()
	for _, output := range windows {
		if output.window != nil {
			output.window.Perform(system.ActionClose)
		}
	}
	if m.decoder != nil {
		m.decoder.Close()
	}
	if m.audio != nil {
		m.audio.Close()
	}
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
	nativeHandle    uintptr
	routed          bool
	displayMissing  bool
	lastGeometry    [4]int
	heldFrame       image.Image
	routeMu         sync.Mutex
	lastSequence    uint64
	geometryUpdates chan [4]int
}

func (m *Manager) outputIDsWithConfiguredStages(outputIDs []string) []string {
	seen := make(map[string]struct{}, len(outputIDs))
	result := make([]string, 0, len(outputIDs)+len(m.settings.Snapshot().VideoOutputs))
	for _, outputID := range outputIDs {
		if outputID != "" {
			if _, exists := seen[outputID]; !exists {
				seen[outputID], result = struct{}{}, append(result, outputID)
			}
		}
	}
	for _, output := range m.settings.Snapshot().VideoOutputs {
		if _, exists := seen[output.Stage]; !exists {
			seen[output.Stage], result = struct{}{}, append(result, output.Stage)
		}
	}
	return result
}

type outputTransition struct {
	event   playback.Event
	stage   string
	started time.Time
}

func (o *outputWindow) run() {
	o.geometryUpdates = make(chan [4]int, 1)
	defer func() {
		o.manager.removed(o.id)
		if o.reopening || o.manager.shouldRecoverOutput(o.id) {
			time.AfterFunc(250*time.Millisecond, func() { o.manager.ensureOutput(o.id) })
		}
	}()
	log.Printf("opening media output %q", o.id)
	route := o.route()
	o.window = new(app.Window)
	o.window.Option(app.Title(o.id), app.Size(unit.Dp(route.Width), unit.Dp(route.Height)), app.MinSize(unit.Dp(320), unit.Dp(180)))
	events, unsubscribe := o.manager.engine.Subscribe(o.id)
	defer unsubscribe()
	pending := make(chan playback.Event, 64)
	done := make(chan struct{})
	defer close(done)
	go o.persistGeometryLoop(done)
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
		case app.ViewEvent:
			if handle := platformViewHandle(event); handle != 0 {
				o.routeMu.Lock()
				o.nativeHandle = handle
				o.routeMu.Unlock()
				// SetWindowPos sends synchronous messages to the Win32 window
				// thread. Calling it from the Gio event handler deadlocks because
				// that thread cannot dispatch the messages until Event returns.
				go o.applyRoute(false)
			}
		case app.ConfigEvent:
			if event.Config.Mode == app.Windowed && !o.route().Fullscreen {
				o.persistGeometry()
			}
		case app.FrameEvent:
			gtx := app.NewContext(&ops, event)
			if o.route().HideCursor {
				pointer.CursorNone.Add(gtx.Ops)
			}
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
				if click.NumClicks == 2 && !o.route().LockedFullscreen {
					o.routeMu.Lock()
					o.fullscreen = !o.fullscreen
					fullscreen := o.fullscreen
					o.routeMu.Unlock()
					if fullscreen {
						o.window.Option(app.Fullscreen.Option())
					} else {
						o.window.Option(app.Windowed.Option())
					}
				}
			}
			paint.Fill(gtx.Ops, palette.Black)
			o.clickable.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return o.layout(gtx)
			})
			event.Frame(gtx.Ops)
		}
	}
}

func (o *outputWindow) route() config.VideoOutput {
	return config.VideoOutputFor(o.manager.settings.Snapshot(), o.id)
}

func (o *outputWindow) applyRoute(force bool) {
	o.routeMu.Lock()
	if o.window == nil || o.nativeHandle == 0 || (o.routed && !force) {
		o.routeMu.Unlock()
		return
	}
	handle := o.nativeHandle
	o.routed = true
	o.routeMu.Unlock()
	route := o.route()
	found := platformPlaceWindow(handle, route, o.manager.currentDisplays())
	o.routeMu.Lock()
	o.displayMissing = route.DisplayID != "" && !found
	o.routeMu.Unlock()
	if route.Fullscreen {
		o.routeMu.Lock()
		o.fullscreen = true
		o.routeMu.Unlock()
		o.window.Option(app.Fullscreen.Option())
	} else {
		o.routeMu.Lock()
		o.fullscreen = false
		o.routeMu.Unlock()
		o.window.Option(app.Windowed.Option())
	}
	o.window.Invalidate()
}

func (o *outputWindow) persistGeometry() {
	displays := o.manager.currentDisplays()
	if len(displays) == 0 {
		return
	}
	display, _ := resolveDisplayForGeometry(o.route().DisplayID, displays)
	o.routeMu.Lock()
	handle := o.nativeHandle
	o.routeMu.Unlock()
	x, y, width, height, ok := platformWindowGeometry(handle, display)
	geometry := [4]int{x, y, width, height}
	if !ok || geometry == o.lastGeometry {
		return
	}
	o.lastGeometry = geometry
	select {
	case o.geometryUpdates <- geometry:
	default:
		select {
		case <-o.geometryUpdates:
		default:
		}
		select {
		case o.geometryUpdates <- geometry:
		default:
		}
	}
}

func (o *outputWindow) persistGeometryLoop(done <-chan struct{}) {
	for {
		var geometry [4]int
		select {
		case geometry = <-o.geometryUpdates:
		case <-done:
			return
		}
		timer := time.NewTimer(250 * time.Millisecond)
	debounce:
		for {
			select {
			case geometry = <-o.geometryUpdates:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(250 * time.Millisecond)
			case <-timer.C:
				break debounce
			case <-done:
				timer.Stop()
				return
			}
		}
		if err := o.manager.settings.UpdateVideoOutputGeometry(o.id, geometry[0], geometry[1], geometry[2], geometry[3]); err != nil {
			log.Printf("persist media output %q geometry: %v", o.id, err)
		}
	}
}

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
		instance := instance
		o.start(&instance)
	}
}

func (o *outputWindow) start(instance *playback.Instance) {
	if instance == nil || o.players[instance.ID] != nil {
		return
	}
	player := NewPlayerWithBackend(
		*instance,
		o.manager.settings,
		o.manager.decoder,
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
		barColor := barColor
		children[i] = layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, barColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		})
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}
