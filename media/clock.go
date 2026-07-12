package media

import (
	"sync"
	"time"
)

// PlaybackClock is the monotonic timeline shared by audio, video, fades, and
// playback reporting for one media instance.
type PlaybackClock struct {
	mu       sync.RWMutex
	now      func() time.Time
	position time.Duration
	anchor   time.Time
	running  bool
}

func NewPlaybackClock(position time.Duration) *PlaybackClock {
	return newPlaybackClock(position, time.Now)
}

func newPlaybackClock(position time.Duration, now func() time.Time) *PlaybackClock {
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

func (c *PlaybackClock) Pause() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		c.position += c.now().Sub(c.anchor)
		c.running = false
	}
	return c.position
}

func (c *PlaybackClock) Seek(position time.Duration) {
	c.mu.Lock()
	c.position = max(time.Duration(0), position)
	if c.running {
		c.anchor = c.now()
	}
	c.mu.Unlock()
}

func (c *PlaybackClock) Position() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.running {
		return c.position + c.now().Sub(c.anchor)
	}
	return c.position
}

func (c *PlaybackClock) Running() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.running
}
