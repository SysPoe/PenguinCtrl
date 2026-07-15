package media

import (
	"sync"
	"time"
)

// PlaybackClock is the monotonic timeline shared by audio, video, fades, and
// playback reporting for one media instance.
// TODO(macro): Clock ownership is split — Player constructs/starts/pauses it,
// ffmpegSession.Start and recoverAudio call SetMaster with audio rendered
// position. Make one owner of the timeline (session or player) and expose a
// read-only position to the other so master rebinding during audio recovery
// cannot race player pause/seek generation logic.
type PlaybackClock struct {
	mu       sync.RWMutex
	now      func() time.Time
	position time.Duration
	anchor   time.Time
	running  bool
	master   func() time.Duration
	masterAt time.Duration
}

func NewPlaybackClock(position time.Duration) *PlaybackClock {
	return newPlaybackClock(position, time.Now)
}

func newPlaybackClock(position time.Duration, now func() time.Time) *PlaybackClock {
	// TODO(micro): max(time.Duration(0), position) is repeated in Seek; extract clampNonNegativeDuration helper or use max(0, position) consistently.
	return &PlaybackClock{now: now, position: max(time.Duration(0), position)}
}

func (c *PlaybackClock) Start() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.running {
		c.anchor = c.now()
		c.running = true
	}
	return c.anchor
}

func (c *PlaybackClock) StartedAt() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.anchor
}

func (c *PlaybackClock) Pause() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		c.position = c.positionLocked()
		c.running = false
	}
	return c.position
}

func (c *PlaybackClock) Seek(position time.Duration) {
	c.mu.Lock()
	// TODO(micro): same non-negative clamp as constructor; share one helper.
	c.position = max(time.Duration(0), position)
	c.master = nil
	if c.running {
		c.anchor = c.now()
	}
	c.mu.Unlock()
}

func (c *PlaybackClock) Position() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.positionLocked()
}

func (c *PlaybackClock) positionLocked() time.Duration {
	if c.running {
		if c.master != nil {
			return c.position + max(time.Duration(0), c.master()-c.masterAt)
		}
		return c.position + c.now().Sub(c.anchor)
	}
	return c.position
}

// SetMaster rebases the logical timeline onto a monotonic presentation source.
// Audio uses rendered sample frames; video-only sessions retain the wall clock.
func (c *PlaybackClock) SetMaster(master func() time.Duration) {
	if master == nil {
		return
	}
	c.mu.Lock()
	current := c.positionLocked()
	c.position = current
	c.master = master
	c.masterAt = master()
	c.anchor = c.now()
	c.mu.Unlock()
}

func (c *PlaybackClock) Running() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.running
}
