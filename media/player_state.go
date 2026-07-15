package media

import (
	"image"
	"time"
)

type playerSessionState struct {
	session       PlaybackSession
	clock         *PlaybackClock
	position      time.Duration
	paused        bool
	closed        bool
	muted         bool
	volumeDB      float64
	volumeFadeID  uint64
	generation    int
	started       time.Time
	decodeVisible bool
	initialFadeIn bool
}

type playerPresentationState struct {
	frame         image.Image
	presented     bool
	visualFadeAt  time.Time
	visualFadeFor time.Duration
}
