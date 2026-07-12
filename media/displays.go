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
}

func (m *Manager) VideoDisplays() ([]VideoDisplay, error) {
	displays, err := enumerateVideoDisplays()
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
	m.refreshDisplays(true)
}

func videoOutputWarning(settings config.Settings, displays []VideoDisplay) string {
	available := make(map[string]struct{}, len(displays))
	for _, d := range displays {
		available[d.ID] = struct{}{}
	}
	var missing []string
	for _, output := range settings.VideoOutputs {
		if output.DisplayID != "" {
			if _, ok := available[output.DisplayID]; !ok {
				missing = append(missing, output.Stage)
			}
		}
	}
	if len(missing) == 0 {
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
		fmt.Fprintf(&b, "%s:%d:%d:%d:%d;", d.ID, d.X, d.Y, d.Width, d.Height)
	}
	return b.String()
}
func (m *Manager) refreshDisplays(force bool) {
	displays, err := enumerateVideoDisplays()
	if err != nil {
		return
	}
	signature := displaySignature(displays)
	m.displaysMu.Lock()
	changed := force || signature != m.displaySignature
	m.displays, m.displaySignature = displays, signature
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
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		m.refreshDisplays(false)
	}
}
