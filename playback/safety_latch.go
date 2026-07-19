package playback

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/show"
)

const emergencyResetSafetyReason = "E-STOP active; media outputs are reinitializing"

type safetyLatch struct {
	latched atomic.Bool
	reason  atomic.Value
	resetMu sync.Mutex
	reset   bool
	prior   string
}

func newSafetyLatch() *safetyLatch {
	latch := &safetyLatch{}
	latch.reason.Store("")
	return latch
}

func (s *safetyLatch) active() bool { return s.latched.Load() }

func (s *safetyLatch) activate(reason string) bool {
	if !s.latched.CompareAndSwap(false, true) {
		return false
	}
	s.reason.Store(reason)
	return true
}

func (s *safetyLatch) force(reason string) {
	s.latched.Store(true)
	s.reason.Store(reason)
}

func (s *safetyLatch) acknowledge() {
	s.latched.Store(false)
	s.reason.Store("")
}

func (s *safetyLatch) reasonText() string {
	value, _ := s.reason.Load().(string)
	return value
}

func (s *safetyLatch) beginReset() {
	s.resetMu.Lock()
	if !s.reset {
		s.prior = s.reasonText()
		s.reset = true
	}
	s.resetMu.Unlock()
	s.force(emergencyResetSafetyReason)
}

func (s *safetyLatch) completeReset(err error) (failureReason string, changed bool) {
	if err != nil {
		failureReason = "E-STOP reset failed: " + err.Error()
		s.force(failureReason)
		return failureReason, false
	}
	s.resetMu.Lock()
	prior := s.prior
	s.prior = ""
	s.reset = false
	s.resetMu.Unlock()
	if s.reasonText() != emergencyResetSafetyReason {
		return "", false
	}
	if prior != "" {
		s.force(prior)
	} else {
		s.acknowledge()
	}
	return "", true
}

// BeginEmergencyReset prevents new playback from starting while the media
// manager force-closes and recreates its decoder and audio resources.
func (e *Engine) BeginEmergencyReset() {
	e.safety.beginReset()
	e.StopAll()
	e.changed()
}

// CompleteEmergencyReset rearms playback only when the E-STOP reset succeeded.
// A failed reset remains latched so GO cannot target a broken backend.
func (e *Engine) CompleteEmergencyReset(err error) {
	failureReason, changed := e.safety.completeReset(err)
	if failureReason != "" {
		e.recordOperatorError(operatorlog.ShowStopping, "E-STOP", errors.New(failureReason), show.CueID{}, "")
		return
	}
	if changed {
		e.changed()
	}
}

func (e *Engine) SafetyLatchReason() string { return e.safety.reasonText() }

func (e *Engine) AcknowledgeSafetyLatch() {
	e.safety.acknowledge()
	e.changed()
}
