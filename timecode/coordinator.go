package timecode

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

type Source string
type Policy string
type State string

const (
	SourceInternal Source = "internal"
	SourceLTC      Source = "ltc"
	SourceMTC      Source = "mtc"
	SourceOSC      Source = "osc"

	PolicyHold   Policy = "hold"
	PolicyChase  Policy = "chase"
	PolicyResync Policy = "resync"

	StateStopped       State = "stopped"
	StateRunning       State = "running"
	StateChasing       State = "chasing"
	StateDiscontinuity State = "discontinuity"

	defaultFrameRate      = 30
	defaultJumpTolerance  = 500 * time.Millisecond
	waitUntilPollInterval = 20 * time.Millisecond
)

type Config struct {
	Source        Source
	Policy        Policy
	FrameRate     float64
	JumpTolerance time.Duration
}

type Status struct {
	Source        Source
	Policy        Policy
	State         State
	Position      time.Duration
	Running       bool
	Held          bool
	Generation    uint64
	LastUpdate    time.Time
	Discontinuity time.Duration
}

type Coordinator struct {
	mu         sync.RWMutex
	now        func() time.Time
	config     Config
	position   time.Duration
	anchor     time.Time
	running    bool
	held       bool
	state      State
	generation uint64
	lastUpdate time.Time
	pending    time.Duration
	jump       time.Duration
	notify     chan struct{}
	onJump     func(time.Duration)
}

func New(config Config) *Coordinator {
	return newCoordinator(config, time.Now)
}

func newCoordinator(config Config, now func() time.Time) *Coordinator {
	config = normalizeConfig(config)
	return &Coordinator{now: now, config: config, state: StateStopped, notify: make(chan struct{}, 1)}
}

func normalizeConfig(config Config) Config {
	switch config.Source {
	case SourceLTC, SourceMTC, SourceOSC:
	default:
		config.Source = SourceInternal
	}
	switch config.Policy {
	case PolicyChase, PolicyResync:
	default:
		config.Policy = PolicyHold
	}
	if config.FrameRate <= 0 || math.IsNaN(config.FrameRate) || math.IsInf(config.FrameRate, 0) {
		config.FrameRate = defaultFrameRate
	}
	if config.JumpTolerance <= 0 {
		config.JumpTolerance = defaultJumpTolerance
	}
	return config
}

func (c *Coordinator) Configure(config Config) {
	c.mu.Lock()
	c.config = normalizeConfig(config)
	c.position, c.anchor, c.running, c.held = 0, c.now(), false, false
	c.state, c.pending, c.jump = StateStopped, 0, 0
	c.generation++
	c.mu.Unlock()
	c.signal()
}

func (c *Coordinator) Enabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config.Source != SourceInternal
}

func (c *Coordinator) FrameRate() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config.FrameRate
}

func (c *Coordinator) SetOnDiscontinuity(callback func(time.Duration)) {
	c.mu.Lock()
	c.onJump = callback
	c.mu.Unlock()
}

func (c *Coordinator) Position() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.positionLocked()
}

func (c *Coordinator) positionLocked() time.Duration {
	if c.running && !c.held {
		return c.position + c.now().Sub(c.anchor)
	}
	return c.position
}

func (c *Coordinator) Status() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return Status{Source: c.config.Source, Policy: c.config.Policy, State: c.state, Position: c.positionLocked(), Running: c.running, Held: c.held, Generation: c.generation, LastUpdate: c.lastUpdate, Discontinuity: c.jump}
}

func (c *Coordinator) Update(source Source, position time.Duration, running bool) error {
	if position < 0 {
		return errors.New("timecode position cannot be negative")
	}
	c.mu.Lock()
	if c.config.Source == SourceInternal || source != c.config.Source {
		c.mu.Unlock()
		return fmt.Errorf("timecode source %q is not selected", source)
	}
	now := c.now()
	current := c.positionLocked()
	jump := position - current
	discontinuity := c.running && absDuration(jump) > c.config.JumpTolerance
	callback := c.onJump
	if discontinuity && c.config.Policy == PolicyHold {
		c.position, c.pending, c.jump = current, position, jump
		c.anchor, c.running, c.held, c.state = now, false, true, StateDiscontinuity
	} else {
		c.position, c.anchor, c.running, c.held = position, now, running, false
		c.jump = jump
		switch {
		case discontinuity && c.config.Policy == PolicyChase:
			c.state = StateChasing
		case running:
			c.state = StateRunning
		default:
			c.state = StateStopped
		}
		if discontinuity {
			c.generation++
		}
	}
	c.lastUpdate = now
	c.mu.Unlock()
	c.signal()
	if discontinuity && callback != nil {
		callback(jump)
	}
	return nil
}

func (c *Coordinator) Acknowledge(resync bool) {
	c.mu.Lock()
	if c.held {
		if resync {
			c.position = c.pending
			c.generation++
		}
		c.anchor, c.running, c.held, c.state = c.now(), true, false, StateRunning
		c.pending, c.jump = 0, 0
	}
	c.mu.Unlock()
	c.signal()
}

func (c *Coordinator) WaitUntil(ctx context.Context, target time.Duration) bool {
	for {
		c.mu.RLock()
		position, held := c.positionLocked(), c.held
		c.mu.RUnlock()
		if !held && position >= target {
			return true
		}
		timer := time.NewTimer(waitUntilPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false
		case <-c.notify:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func framePosition(hours, minutes, seconds, frames int, rate float64) (time.Duration, error) {
	if hours < 0 || hours > 23 || minutes < 0 || minutes > 59 || seconds < 0 || seconds > 59 || rate <= 0 || frames < 0 || float64(frames) >= math.Ceil(rate) {
		return 0, errors.New("invalid timecode frame")
	}
	base := time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second
	return base + time.Duration(float64(time.Second)*float64(frames)/rate), nil
}

func (c *Coordinator) signal() {
	select {
	case c.notify <- struct{}{}:
	default:
	}
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}
