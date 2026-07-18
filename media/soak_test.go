package media

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/playback"
)

// TestMediaBackendMultiHourSoak is an opt-in real decoder/device soak. Example:
//
//	$env:CUSUS_MEDIA_SOAK_SOURCE='C:\media\av.mp4'
//	$env:CUSUS_MEDIA_SOAK_DURATION='3h'
//	go test ./media -run MultiHourSoak -count=1 -timeout 4h
func TestMediaBackendMultiHourSoak(t *testing.T) {
	source := os.Getenv("CUSUS_MEDIA_SOAK_SOURCE")
	if source == "" {
		t.Skip("set CUSUS_MEDIA_SOAK_SOURCE to an A/V file")
	}
	duration := 3 * time.Hour
	if raw := os.Getenv("CUSUS_MEDIA_SOAK_DURATION"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			t.Fatalf("invalid CUSUS_MEDIA_SOAK_DURATION: %v", err)
		}
		duration = parsed
	}
	overlaps := 3
	if raw := os.Getenv("CUSUS_MEDIA_SOAK_OVERLAPS"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			t.Fatalf("invalid CUSUS_MEDIA_SOAK_OVERLAPS %q", raw)
		}
		overlaps = parsed
	}

	settings, err := config.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	audio, err := NewAudioSystem(settings)
	if err != nil {
		t.Fatal(err)
	}
	backend := NewFFmpegBackend(settings, audio)
	type active struct {
		session PlaybackSession
		clock   *PlaybackClock
	}
	players := make([]active, overlaps)
	open := func(index int) active {
		instance := playback.Instance{
			ID: "soak-" + strconv.Itoa(index), MediaType: "video", Source: source,
			LevelDB: -24,
		}
		session, err := backend.Open(PlaybackRequest{Instance: instance, RequestedAt: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		if err := session.Preload(context.Background()); err != nil {
			t.Fatal(err)
		}
		clock := NewPlaybackClock(0)
		if err := session.Start(clock); err != nil {
			t.Fatal(err)
		}
		return active{session: session, clock: clock}
	}
	for i := range players {
		players[i] = open(i)
	}
	defer func() {
		for _, player := range players {
			player.session.Close()
		}
	}()

	deadline := time.NewTimer(duration)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second / 60)
	defer ticker.Stop()
	var decoded, dropped, buffering uint64
	for {
		select {
		case <-deadline.C:
			for _, player := range players {
				metrics := player.session.Metrics()
				decoded += metrics.DecodedFrames
				dropped += metrics.DroppedFrames
				buffering += metrics.BufferingCount
				if metrics.Error != "" {
					t.Fatalf("decoder runtime error: %s", metrics.Error)
				}
			}
			if decoded == 0 {
				t.Fatal("soak decoded no video frames")
			}
			t.Logf("duration=%s overlaps=%d decoded=%d dropped=%d buffering=%d", duration, overlaps, decoded, dropped, buffering)
			return
		case <-ticker.C:
			for i := range players {
				select {
				case <-players[i].session.Done():
					metrics := players[i].session.Metrics()
					decoded += metrics.DecodedFrames
					dropped += metrics.DroppedFrames
					buffering += metrics.BufferingCount
					players[i].session.Close()
					players[i] = open(i)
				default:
					players[i].session.Frame(players[i].clock.Position())
				}
			}
		}
	}
}
