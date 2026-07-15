package media

import (
	"image"
	"log"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/playback"
)

type outputWindow struct {
	id              string
	controller      *outputController
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

const (
	outputWindowSettle      = 250 * time.Millisecond
	outputWindowMinWidth    = unit.Dp(320)
	outputWindowMinHeight   = unit.Dp(180)
	outputWindowEventBuffer = 64
)

func (o *outputWindow) run() {
	o.geometryUpdates = make(chan [4]int, 1)
	defer func() {
		o.controller.removed(o.id)
		if o.reopening || o.controller.shouldRecoverOutput(o.id) {
			time.AfterFunc(outputWindowSettle, func() { o.controller.ensureOutput(o.id) })
		}
	}()
	log.Printf("opening media output %q", o.id)
	route := o.route()
	o.window.Option(app.Title(o.id), app.Size(unit.Dp(route.Width), unit.Dp(route.Height)), app.MinSize(outputWindowMinWidth, outputWindowMinHeight))
	events, unsubscribe := o.controller.port.Subscribe(o.id)
	defer unsubscribe()
	pending := make(chan playback.Event, outputWindowEventBuffer)
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

func (o *outputWindow) close() {
	o.routeMu.Lock()
	window := o.window
	o.routeMu.Unlock()
	if window != nil {
		window.Perform(system.ActionClose)
	}
}

func (o *outputWindow) route() config.VideoOutput {
	return config.VideoOutputFor(o.controller.settings.Snapshot(), o.id)
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
	found := platformPlaceWindow(handle, route, o.controller.topology.currentDisplays())
	o.routeMu.Lock()
	o.displayMissing = route.DisplayID != "" && !found
	o.routeMu.Unlock()
	o.routeMu.Lock()
	o.fullscreen = route.Fullscreen
	o.routeMu.Unlock()
	if route.Fullscreen {
		o.window.Option(app.Fullscreen.Option())
	} else {
		o.window.Option(app.Windowed.Option())
	}
	o.window.Invalidate()
}

func (o *outputWindow) persistGeometry() {
	displays := o.controller.topology.currentDisplays()
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
		timer := time.NewTimer(outputWindowSettle)
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
				timer.Reset(outputWindowSettle)
			case <-timer.C:
				break debounce
			case <-done:
				timer.Stop()
				return
			}
		}
		if err := o.controller.settings.UpdateVideoOutputGeometry(o.id, geometry[0], geometry[1], geometry[2], geometry[3]); err != nil {
			log.Printf("persist media output %q geometry: %v", o.id, err)
		}
	}
}
