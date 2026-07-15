package playback

import (
	"fmt"
	"strings"
	"sync"

	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/show"
)

type commandIntent uint8

const (
	liveCommand commandIntent = iota
	previewCommand
)

func (i commandIntent) preview() bool { return i == previewCommand }

type blockerPolicy uint8

const (
	rejectBlockers blockerPolicy = iota
	overrideBlockers
)

type admissionRequest struct {
	cue     show.Cue
	origin  string
	intent  commandIntent
	blocker blockerPolicy
}

type admissionGates struct {
	mu        sync.RWMutex
	authority func() error
	preflight func(show.Cue) error
}

func (a *admissionGates) setAuthority(gate func() error) {
	a.mu.Lock()
	a.authority = gate
	a.mu.Unlock()
}

func (a *admissionGates) setPreflight(gate func(show.Cue) error) {
	a.mu.Lock()
	a.preflight = gate
	a.mu.Unlock()
}

func (a *admissionGates) authorityGate() func() error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.authority
}

func (a *admissionGates) preflightGate() func(show.Cue) error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.preflight
}

func (e *Engine) SetPreflightGate(gate func(show.Cue) error) {
	e.admission.setPreflight(gate)
}

// SetAuthorityGate installs a non-overridable command-ownership barrier. It
// is checked both when a cue is accepted and immediately before execution.
func (e *Engine) SetAuthorityGate(gate func() error) {
	e.admission.setAuthority(gate)
}

func (e *Engine) admitCommand(request admissionRequest) error {
	if e.safety.active() {
		err := fmt.Errorf("playback safety latch is active: %s", e.SafetyLatchReason())
		if !request.intent.preview() {
			e.recordCueError(request.cue, request.origin, err)
		}
		return err
	}
	if request.intent.preview() {
		return nil
	}
	if err := e.checkAuthority(request.cue, request.origin); err != nil {
		return err
	}
	if gate := e.admission.preflightGate(); gate != nil {
		if err := gate(request.cue); err != nil {
			if request.blocker != overrideBlockers {
				e.recordCueError(request.cue, request.origin+" · preflight", err)
				return err
			}
			if log := e.operatorLogStore(); log != nil {
				log.Add(operatorlog.Warning, request.origin+" · preflight override", "GO override accepted despite: "+err.Error(), request.cue.ID, request.cue.CueNumber)
			}
		}
	}
	problems := e.CueProblems(request.cue)
	blockers := problemMessages(problems, show.ProblemBlocker)
	cautions := problemMessages(problems, show.ProblemCaution)
	if len(blockers) > 0 && request.blocker != overrideBlockers {
		err := fmt.Errorf("cue blocked: %s", strings.Join(blockers, "; "))
		e.recordCueError(request.cue, request.origin+" · validation", err)
		return err
	}
	if log := e.operatorLogStore(); len(blockers) > 0 && request.blocker == overrideBlockers && log != nil {
		log.Add(operatorlog.Warning, request.origin+" · override", "GO override accepted despite: "+strings.Join(blockers, "; "), request.cue.ID, request.cue.CueNumber)
	}
	if log := e.operatorLogStore(); len(cautions) > 0 && log != nil {
		log.Add(operatorlog.Warning, request.origin+" · caution", strings.Join(cautions, "; "), request.cue.ID, request.cue.CueNumber)
	}
	return nil
}

func (e *Engine) checkAuthority(cue show.Cue, origin string) error {
	gate := e.admission.authorityGate()
	if gate == nil {
		return nil
	}
	if err := gate(); err != nil {
		e.recordCueError(cue, origin+" · command authority", err)
		return err
	}
	return nil
}
