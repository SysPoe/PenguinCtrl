package playback

import (
	"errors"
	"fmt"
	"strings"

	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/remote"
	"github.com/syspoe/cusus/show"
)

type cueExecutor interface {
	execute(command) (keepRun bool, err error)
}

type cueExecutorBinding struct {
	executor               cueExecutor
	advanceBeforeExecution bool
}

type cueExecutorSet struct {
	byType map[show.CueType]cueExecutorBinding
}

func newCueExecutorSet(engine *Engine) *cueExecutorSet {
	media := mediaCueExecutor{engine: engine}
	return &cueExecutorSet{byType: map[show.CueType]cueExecutorBinding{
		show.CueTypeSound:         {executor: media},
		show.CueTypeVideo:         {executor: media},
		show.CueTypeImage:         {executor: media},
		show.CueTypeRemote:        {executor: remoteCueExecutor{engine: engine}},
		show.CueTypeWait:          {executor: waitCueExecutor{engine: engine}, advanceBeforeExecution: true},
		show.CueTypeMediaControl:  {executor: mediaControlCueExecutor{engine: engine}},
		show.CueTypeOutputControl: {executor: outputControlCueExecutor{engine: engine}},
	}}
}

func (executors *cueExecutorSet) forType(cueType show.CueType) (cueExecutorBinding, bool) {
	binding, ok := executors.byType[cueType]
	return binding, ok
}

type mediaCueExecutor struct {
	engine *Engine
}

func (executor mediaCueExecutor) execute(next command) (bool, error) {
	err := executor.engine.startMedia(next)
	return err == nil, err
}

type remoteCueExecutor struct {
	engine *Engine
}

func (executor remoteCueExecutor) execute(next command) (bool, error) {
	if next.cue.Play.Remote == nil {
		return false, errors.New("remote cue has no remote action")
	}
	var result remote.DispatchResult
	dispatch := func() error {
		var err error
		result, err = executor.engine.remote.DispatchWithResult(executor.engine.ctx, *next.cue.Play.Remote, next.cue)
		return err
	}
	executor.engine.mu.RLock()
	authorize := executor.engine.remoteAuthority
	executor.engine.mu.RUnlock()
	var err error
	if authorize != nil {
		err = authorize(dispatch)
	} else {
		err = dispatch()
	}
	if log := executor.engine.operatorLogStore(); err == nil && log != nil {
		message := remoteDispatchMessage(result, false)
		severity := operatorlog.Warning
		if result.Acknowledged {
			message = remoteDispatchMessage(result, true)
			severity = operatorlog.Info
		}
		log.Add(severity, next.origin+" · remote result", message, next.cue.ID, next.cue.CueNumber)
	}
	return false, err
}

type waitCueExecutor struct {
	engine *Engine
}

func (executor waitCueExecutor) execute(next command) (bool, error) {
	return false, executor.engine.executeWait(next.cue, next.run.ctx)
}

type mediaControlCueExecutor struct {
	engine *Engine
}

func (executor mediaControlCueExecutor) execute(next command) (bool, error) {
	return false, executor.engine.executeMediaControl(next.cue, next.run.ctx)
}

type outputControlCueExecutor struct {
	engine *Engine
}

func (executor outputControlCueExecutor) execute(next command) (bool, error) {
	return false, executor.engine.executeOutputControl(next.cue, next.run.ctx)
}

func remoteDispatchMessage(result remote.DispatchResult, acknowledged bool) string {
	protocols := make([]string, 0, len(result.Protocols))
	for _, protocol := range result.Protocols {
		switch protocol {
		case show.RemoteProtocolOSC:
			protocols = append(protocols, "OSC")
		case show.RemoteProtocolERC:
			protocols = append(protocols, "ERC")
		default:
			protocols = append(protocols, fmt.Sprintf("protocol %d", protocol))
		}
	}
	transport := ""
	if len(protocols) > 0 {
		transport = " via " + strings.Join(protocols, "/")
	}
	if acknowledged {
		return "Command sent" + transport + " and acknowledged by the configured idempotent relay"
	}
	return "Command sent" + transport + "; UDP delivery is unconfirmed"
}
