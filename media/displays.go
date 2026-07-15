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

func (m *Manager) VideoDisplays() ([]VideoDisplay, error) {
	m.displaysMu.RLock()
	displays, err := append([]VideoDisplay(nil), m.displays...), m.displaysErr
	m.displaysMu.RUnlock()
	sort.SliceStable(displays, func(i, j int) bool {
		if displays[i].Primary != displays[j].Primary {
			return displays[i].Primary
		}
		return displays[i].Name < displays[j].Name
	})
	return displays, err
}

func (m *Manager) VideoOutputWarning() string {
	m.displayStatusMu.Lock()
	defer m.displayStatusMu.Unlock()
	if !m.lastDisplayCheck.IsZero() && time.Since(m.lastDisplayCheck) < time.Second {
		return m.videoOutputStatus
	}
	m.lastDisplayCheck = time.Now()
	m.videoOutputStatus = videoOutputWarning(m.settings.Snapshot(), m.currentDisplays())
	return m.videoOutputStatus
}

func (m *Manager) RefreshVideoOutputStatus() {
	m.displayStatusMu.Lock()
	m.lastDisplayCheck = time.Time{}
	m.displayStatusMu.Unlock()
	select {
	case m.displayRefresh <- struct{}{}:
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
		// TODO(micro): refresh-mismatch is only checked in the else of !DisplayConfirmed; confirmed mappings with wrong refresh are never reported -- invert so refresh is independent of confirmation.
		} else if display, ok := available[output.DisplayID]; ok && output.ExpectedRefresh > 0 && display.RefreshRate != output.ExpectedRefresh {
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

func (m *Manager) currentDisplays() []VideoDisplay {
	m.displaysMu.RLock()
	defer m.displaysMu.RUnlock()
	return append([]VideoDisplay(nil), m.displays...)
}
func displaySignature(ds []VideoDisplay) string {
	var b strings.Builder
	for _, d := range ds {
		// TODO(micro): signature omits Primary and Name, which are used for sorting/warnings; include them or document why topology-only is enough.
		fmt.Fprintf(&b, "%s:%d:%d:%d:%d:%d:%d;", d.ID, d.X, d.Y, d.Width, d.Height, d.RefreshRate, d.DPI)
	}
	return b.String()
}
func (m *Manager) refreshDisplays(force bool) {
	displays, err := enumerateVideoDisplays()
	if err != nil {
		m.displaysMu.Lock()
		m.displaysErr = err
		m.displaysMu.Unlock()
		return
	}
	signature := displaySignature(displays)
	m.displaysMu.Lock()
	changed := force || signature != m.displaySignature
	m.displays, m.displaySignature, m.displaysErr = displays, signature, nil
	m.displaysMu.Unlock()
	if !changed {
		return
	}
	m.displayStatusMu.Lock()
	m.lastDisplayCheck = time.Time{}
	m.displayStatusMu.Unlock()
	m.mu.Lock()
	outputs := make([]*outputWindow, 0, len(m.windows))
	for _, output := range m.windows {
		outputs = append(outputs, output)
	}
	m.mu.Unlock()
	for _, output := range outputs {
		output.applyRoute(true)
	}
}
func (m *Manager) monitorDisplays() {
	// TODO(micro): 2s poll interval is duplicated with monitorAudioDevices; extract a shared devicePollInterval constant.
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		m.refreshDisplays(false)
		select {
		case <-m.ctx.Done():
			return
		case <-m.displayRefresh:
		case <-ticker.C:
		}
	}
}
