package media

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/syspoe/cusus/config"
)

type VideoDisplay struct {
	ID, Name            string
	Primary             bool
	X, Y, Width, Height int
	RefreshRate, DPI    int
}

const defaultDisplayDPI = 96

func (topology *deviceTopology) videoDisplays() ([]VideoDisplay, error) {
	topology.displaysMu.RLock()
	displays, err := append([]VideoDisplay(nil), topology.displays...), topology.displaysErr
	topology.displaysMu.RUnlock()
	sort.SliceStable(displays, func(i, j int) bool {
		if displays[i].Primary != displays[j].Primary {
			return displays[i].Primary
		}
		return displays[i].Name < displays[j].Name
	})
	return displays, err
}

func (topology *deviceTopology) videoWarning() string {
	topology.displayStatusMu.Lock()
	defer topology.displayStatusMu.Unlock()
	if !topology.lastDisplayCheck.IsZero() && time.Since(topology.lastDisplayCheck) < time.Second {
		return topology.videoStatus
	}
	topology.lastDisplayCheck = time.Now()
	topology.videoStatus = videoOutputWarning(topology.settings.Snapshot(), topology.currentDisplays())
	return topology.videoStatus
}

func (topology *deviceTopology) refreshVideoStatus() {
	topology.displayStatusMu.Lock()
	topology.lastDisplayCheck = time.Time{}
	topology.displayStatusMu.Unlock()
	select {
	case topology.displayRefresh <- struct{}{}:
	default:
	}
}

func videoOutputWarning(settings config.Settings, displays []VideoDisplay) string {
	available := make(map[string]VideoDisplay, len(displays))
	for _, d := range displays {
		available[d.ID] = d
	}
	var missing []string
	var unconfirmed []string
	var refreshMismatch []string
	for _, output := range settings.VideoOutputs {
		if output.DisplayID != "" {
			if _, ok := available[output.DisplayID]; !ok {
				missing = append(missing, output.Stage)
			}
		}
		if output.DisplayID != "" && !output.DisplayConfirmed {
			unconfirmed = append(unconfirmed, output.Stage)
		}
		if display, ok := available[output.DisplayID]; ok && output.ExpectedRefresh > 0 && display.RefreshRate != output.ExpectedRefresh {
			refreshMismatch = append(refreshMismatch, fmt.Sprintf("%s expects %d Hz but found %d Hz", output.Stage, output.ExpectedRefresh, display.RefreshRate))
		}
	}
	if len(missing) == 0 {
		if len(unconfirmed) > 0 {
			return fmt.Sprintf("Display mappings for stages %s require operator confirmation before show mode.", strings.Join(unconfirmed, ", "))
		}
		if len(refreshMismatch) > 0 {
			return "Display refresh mismatch: " + strings.Join(refreshMismatch, "; ")
		}
		return ""
	}
	if len(missing) == 1 {
		return fmt.Sprintf("Stage %q is assigned to a disconnected display and is temporarily on the primary display.", missing[0])
	}
	return fmt.Sprintf("Stages %s are assigned to disconnected displays and are temporarily on the primary display.", strings.Join(missing, ", "))
}

func (topology *deviceTopology) currentDisplays() []VideoDisplay {
	topology.displaysMu.RLock()
	defer topology.displaysMu.RUnlock()
	return append([]VideoDisplay(nil), topology.displays...)
}
func displaySignature(ds []VideoDisplay) string {
	var b strings.Builder
	for _, d := range ds {
		fmt.Fprintf(&b, "%s:%q:%t:%d:%d:%d:%d:%d:%d;", d.ID, d.Name, d.Primary, d.X, d.Y, d.Width, d.Height, d.RefreshRate, d.DPI)
	}
	return b.String()
}
func (topology *deviceTopology) refreshDisplays(force bool) {
	displays, err := enumerateVideoDisplays()
	if err != nil {
		topology.displaysMu.Lock()
		topology.displaysErr = err
		topology.displaysMu.Unlock()
		return
	}
	signature := displaySignature(displays)
	topology.displaysMu.Lock()
	changed := force || signature != topology.displaySignature
	topology.displays, topology.displaySignature, topology.displaysErr = displays, signature, nil
	topology.displaysMu.Unlock()
	if !changed {
		return
	}
	topology.displayStatusMu.Lock()
	topology.lastDisplayCheck = time.Time{}
	topology.displayStatusMu.Unlock()
	if topology.onDisplaysChanged != nil {
		topology.onDisplaysChanged()
	}
}
func (topology *deviceTopology) monitorDisplays() {
	ticker := time.NewTicker(devicePollInterval)
	defer ticker.Stop()
	for {
		topology.refreshDisplays(false)
		select {
		case <-topology.ctx.Done():
			return
		case <-topology.displayRefresh:
		case <-ticker.C:
		}
	}
}
