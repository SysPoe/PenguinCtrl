package timecode

import "sync"

// LTCAdapter converts complete LTC frames into source-agnostic timeline
// updates.
type LTCAdapter struct{ coordinator *Coordinator }

// NewLTCAdapter binds an LTC frame decoder to coordinator.
func NewLTCAdapter(coordinator *Coordinator) *LTCAdapter {
	return &LTCAdapter{coordinator: coordinator}
}

// IngestFrame converts one LTC frame to a running timeline position.
func (a *LTCAdapter) IngestFrame(hours, minutes, seconds, frames int, rate float64) error {
	position, err := framePosition(hours, minutes, seconds, frames, rate)
	if err != nil {
		return err
	}
	return a.coordinator.Update(SourceLTC, position, true)
}

// MTCAdapter reassembles MTC quarter frames before publishing a timeline
// update.
type MTCAdapter struct {
	coordinator *Coordinator
	mu          sync.Mutex
	parts       [8]byte
	mask        uint8
}

// NewMTCAdapter binds an MTC quarter-frame decoder to coordinator.
func NewMTCAdapter(coordinator *Coordinator) *MTCAdapter {
	return &MTCAdapter{coordinator: coordinator}
}

// IngestQuarterFrame accepts one MTC quarter-frame message.
func (a *MTCAdapter) IngestQuarterFrame(data byte) error {
	part, value := (data>>4)&0x07, data&0x0f
	a.mu.Lock()
	a.parts[part], a.mask = value, a.mask|(1<<part)
	complete := a.mask == 0xff
	values := a.parts
	if complete {
		a.mask = 0
	}
	a.mu.Unlock()
	if !complete {
		return nil
	}
	frames := int(values[0] | (values[1]&0x1)<<4)
	seconds := int(values[2] | (values[3]&0x3)<<4)
	minutes := int(values[4] | (values[5]&0x3)<<4)
	hours := int(values[6] | (values[7]&0x1)<<4)
	rate := []float64{24, 25, 29.97, 30}[(values[7]>>1)&0x3]
	position, err := framePosition(hours, minutes, seconds, frames, rate)
	if err != nil {
		return err
	}
	return a.coordinator.Update(SourceMTC, position, true)
}
