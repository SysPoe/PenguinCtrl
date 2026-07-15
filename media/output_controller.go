package media

import (
	"sync"

	"gioui.org/app"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/playback"
)

// outputController owns stage-window membership and recovery. Windows depend
// on playback.MediaOutput, never on playback.Engine's command surface.
type outputController struct {
	port     playback.MediaOutput
	settings *config.Store
	runtime  *mediaRuntime
	topology *deviceTopology
	mu       sync.Mutex
	windows  map[string]*outputWindow
	desired  map[string]struct{}
	closed   bool
}

func newOutputController(port playback.MediaOutput, settings *config.Store, runtime *mediaRuntime, topology *deviceTopology) *outputController {
	return &outputController{port: port, settings: settings, runtime: runtime, topology: topology, windows: map[string]*outputWindow{}, desired: map[string]struct{}{}}
}

func (controller *outputController) ensure(outputIDs []string) {
	outputIDs = controller.outputIDsWithConfiguredStages(outputIDs)
	controller.mu.Lock()
	for _, outputID := range outputIDs {
		controller.desired[outputID] = struct{}{}
	}
	controller.mu.Unlock()
	for _, outputID := range outputIDs {
		controller.ensureOutput(outputID)
	}
}

func (controller *outputController) sync(outputIDs []string) {
	outputIDs = controller.outputIDsWithConfiguredStages(outputIDs)
	desired := make(map[string]struct{}, len(outputIDs))
	for _, outputID := range outputIDs {
		desired[outputID] = struct{}{}
	}
	controller.mu.Lock()
	if controller.closed {
		controller.mu.Unlock()
		return
	}
	controller.desired = desired
	var stale []*outputWindow
	for outputID, output := range controller.windows {
		if _, keep := desired[outputID]; !keep {
			stale = append(stale, output)
		}
	}
	controller.mu.Unlock()
	for _, outputID := range outputIDs {
		controller.ensureOutput(outputID)
	}
	closeOutputWindows(stale)
}

func (controller *outputController) ensureOutput(outputID string) {
	if outputID == "" {
		return
	}
	controller.mu.Lock()
	if controller.closed {
		controller.mu.Unlock()
		return
	}
	if _, exists := controller.windows[outputID]; exists {
		controller.mu.Unlock()
		return
	}
	output := &outputWindow{id: outputID, controller: controller, window: new(app.Window), theme: material.NewTheme()}
	output.session = newStageSession(output)
	controller.windows[outputID] = output
	controller.mu.Unlock()
	go output.run()
}

func (controller *outputController) removed(outputID string) {
	controller.mu.Lock()
	delete(controller.windows, outputID)
	controller.mu.Unlock()
}

func (controller *outputController) shouldRecoverOutput(outputID string) bool {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if controller.closed {
		return false
	}
	_, desired := controller.desired[outputID]
	return desired
}

func (controller *outputController) refreshRoutes() {
	controller.mu.Lock()
	windows := make([]*outputWindow, 0, len(controller.windows))
	for _, output := range controller.windows {
		windows = append(windows, output)
	}
	controller.mu.Unlock()
	for _, output := range windows {
		output.applyRoute(true)
	}
}

func (controller *outputController) restart() {
	controller.mu.Lock()
	windows := make([]*outputWindow, 0, len(controller.windows))
	for _, output := range controller.windows {
		windows = append(windows, output)
	}
	controller.mu.Unlock()
	closeOutputWindows(windows)
}

func (controller *outputController) close() {
	controller.mu.Lock()
	if controller.closed {
		controller.mu.Unlock()
		return
	}
	controller.closed = true
	controller.desired = map[string]struct{}{}
	windows := make([]*outputWindow, 0, len(controller.windows))
	for _, output := range controller.windows {
		windows = append(windows, output)
	}
	controller.mu.Unlock()
	closeOutputWindows(windows)
}

func closeOutputWindows(windows []*outputWindow) {
	for _, output := range windows {
		output.close()
	}
}

func (controller *outputController) outputIDsWithConfiguredStages(outputIDs []string) []string {
	settings := controller.settings.Snapshot()
	seen := make(map[string]struct{}, len(outputIDs))
	result := make([]string, 0, len(outputIDs)+len(settings.VideoOutputs))
	for _, outputID := range outputIDs {
		if outputID != "" {
			if _, exists := seen[outputID]; !exists {
				seen[outputID], result = struct{}{}, append(result, outputID)
			}
		}
	}
	for _, output := range settings.VideoOutputs {
		if _, exists := seen[output.Stage]; !exists {
			seen[output.Stage], result = struct{}{}, append(result, output.Stage)
		}
	}
	return result
}
