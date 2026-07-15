package media

import (
	"log"
	"time"

	"gioui.org/app"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/playback"
)

// TODO(macro): outputWindow.run is a second Gio app loop (window events, geometry
// persistence, engine subscription, player reconcile) living beside the operator
// window_loop. Extract a StageWindowController so output lifecycle, recovery
// reopen, and layout are not an anonymous method chain on a type declared in
// manager.go.
func (o *outputWindow) run() {
	o.geometryUpdates = make(chan [4]int, 1)
	defer func() {
		o.manager.removed(o.id)
		if o.reopening || o.manager.shouldRecoverOutput(o.id) {
			// TODO(micro): 250ms reopen delay is duplicated with geometry debounce below; name a shared outputWindowSettle constant.
			time.AfterFunc(250*time.Millisecond, func() { o.manager.ensureOutput(o.id) })
		}
	}()
	log.Printf("opening media output %q", o.id)
	route := o.route()
	o.window = new(app.Window)
	// TODO(micro): min window size 320x180 is magic; name constants.
	o.window.Option(app.Title(o.id), app.Size(unit.Dp(route.Width), unit.Dp(route.Height)), app.MinSize(unit.Dp(320), unit.Dp(180)))
	events, unsubscribe := o.manager.engine.Subscribe(o.id)
	defer unsubscribe()
	// TODO(micro): pending event buffer size 64 is magic; name a constant.
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
	// TODO(micro): both branches only differ by fullscreen bool and Option; set o.fullscreen once and pick Fullscreen/Windowed.
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
