package media

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/playback"
	"github.com/syspoe/cusus/show"
)

type Player struct {
	instance    playback.Instance
	settings    *config.Store
	audioSystem *AudioSystem
	window      *app.Window
	report      func(string)
	duration    func(int64)

	mu           sync.RWMutex
	frame        image.Image
	audio        *devicePlayer
	videoCommand *exec.Cmd
	audioCommand *exec.Cmd
	position     time.Duration
	startedAt    time.Time
	paused       bool
	closed       bool
	muted        bool
	volumeDB     float64
	volumeFadeID uint64
	generation   int
	started      time.Time
}

func NewPlayer(instance playback.Instance, settings *config.Store, audio *AudioSystem, window *app.Window, report func(string), duration func(int64)) *Player {
	return &Player{instance: instance, settings: settings, audioSystem: audio, window: window, report: report, duration: duration, volumeDB: instance.LevelDB}
}

func (p *Player) MediaType() string { return p.instance.MediaType }

func (p *Player) StartedAt() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.started
}

func (p *Player) Start() error {
	p.mu.Lock()
	p.started = time.Now()
	p.position = time.Duration(max(0, p.instance.ClipStartMs)) * time.Millisecond
	p.mu.Unlock()
	p.discoverDuration()
	var err error
	if p.instance.MediaType == "image" {
		err = p.loadImage()
	} else {
		err = p.restart(p.position)
	}
	if err == nil && p.instance.FadeInMs > 0 {
		go func() {
			timer := time.NewTimer(time.Duration(p.instance.FadeInMs) * time.Millisecond)
			defer timer.Stop()
			<-timer.C
			p.mu.RLock()
			active := !p.closed
			p.mu.RUnlock()
			if active {
				p.report("fade-in-complete")
			}
		}()
	}
	return err
}

func (p *Player) discoverDuration() {
	if p.instance.DurationMs > 0 || (p.instance.MediaType != "audio" && p.instance.MediaType != "video") {
		return
	}
	mediaDurationMs, err := ProbeDurationMs(p.settings.Snapshot().FFmpegPath, p.instance.Source)
	if err != nil {
		return
	}
	durationMs := mediaDurationMs - max(0, p.instance.ClipStartMs)
	if p.instance.ClipEndMs > p.instance.ClipStartMs {
		durationMs = p.instance.ClipEndMs - p.instance.ClipStartMs
	}
	if durationMs <= 0 {
		return
	}
	p.instance.DurationMs = durationMs
	if p.duration != nil {
		p.duration(durationMs)
	}
}

func (p *Player) loadImage() error {
	path, err := sourcePath(p.instance.Source)
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	img, _, err := image.Decode(bufio.NewReader(file))
	if err != nil {
		return err
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return errors.New("player is closed")
	}
	p.frame = img
	p.mu.Unlock()
	p.window.Invalidate()
	return nil
}

func (p *Player) restart(position time.Duration) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return errors.New("player is closed")
	}
	p.stopCommandsLocked()
	p.generation++
	generation := p.generation
	p.position = position
	p.startedAt = time.Now()
	p.paused = false
	p.mu.Unlock()

	if p.instance.MediaType == "video" {
		if err := p.startVideo(position, generation); err != nil {
			return err
		}
	}
	if p.instance.MediaType == "audio" || p.instance.MediaType == "video" {
		if err := p.startAudio(position, generation); err != nil && p.instance.MediaType == "audio" {
			return err
		}
	}
	return nil
}

func (p *Player) startVideo(position time.Duration, generation int) error {
	path, err := sourcePath(p.instance.Source)
	if err != nil {
		return err
	}
	width, height, err := probeVideo(p.settings.Snapshot().FFmpegPath, path)
	if err != nil {
		return err
	}
	args := mediaInputArgs(position, p.instance.ClipEndMs)
	args = append(args, "-i", path, "-an", "-f", "rawvideo", "-pix_fmt", "rgba", "pipe:1")
	command := exec.Command(p.settings.Snapshot().FFmpegPath, args...)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		return err
	}
	p.mu.Lock()
	p.videoCommand = command
	p.mu.Unlock()
	go p.readFrames(command, stdout, width, height, generation)
	return nil
}

func (p *Player) readFrames(command *exec.Cmd, reader io.Reader, width, height, generation int) {
	frameSize := width * height * 4
	buffer := make([]byte, frameSize)
	for {
		if _, err := io.ReadFull(reader, buffer); err != nil {
			break
		}
		frame := image.NewRGBA(image.Rect(0, 0, width, height))
		copy(frame.Pix, buffer)
		p.mu.Lock()
		if p.closed || p.generation != generation {
			p.mu.Unlock()
			break
		}
		p.frame = frame
		p.mu.Unlock()
		p.window.Invalidate()
	}
	_ = command.Wait()
	p.mu.RLock()
	ended := !p.closed && !p.paused && p.generation == generation
	p.mu.RUnlock()
	if ended {
		p.report("ended")
	}
}

func (p *Player) startAudio(position time.Duration, generation int) error {
	if p.audioSystem == nil {
		return errors.New("audio output is unavailable")
	}
	path, err := sourcePath(p.instance.Source)
	if err != nil {
		return err
	}
	args := mediaInputArgs(position, p.instance.ClipEndMs)
	args = append(args, "-i", path, "-vn", "-f", "s16le", "-ar", "48000", "-ac", "2", "pipe:1")
	command := exec.Command(p.settings.Snapshot().FFmpegPath, args...)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		return err
	}
	player, err := p.audioSystem.NewPlayer(stdout, p.instance.Preview)
	if err != nil {
		_ = command.Process.Kill()
		return err
	}
	player.SetVolume(dbVolume(p.volumeDB, p.muted))
	p.mu.Lock()
	p.audioCommand, p.audio = command, player
	p.mu.Unlock()
	go func() {
		_ = command.Wait()
		p.mu.RLock()
		ended := p.instance.MediaType == "audio" && !p.closed && !p.paused && p.generation == generation
		p.mu.RUnlock()
		if ended {
			p.report("ended")
		}
	}()
	return nil
}

func mediaInputArgs(position time.Duration, clipEndMs int64) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-re"}
	if position > 0 {
		args = append(args, "-ss", strconv.FormatFloat(position.Seconds(), 'f', 3, 64))
	}
	if clipEndMs > 0 && time.Duration(clipEndMs)*time.Millisecond > position {
		args = append(args, "-t", strconv.FormatFloat((time.Duration(clipEndMs)*time.Millisecond-position).Seconds(), 'f', 3, 64))
	}
	return args
}

func (p *Player) Control(event playback.Event) {
	switch event.Control {
	case "pause":
		p.pause()
	case "resume":
		p.resume()
	case "seek":
		if event.PositionMs != nil {
			_ = p.restart(time.Duration(max(0, *event.PositionMs)) * time.Millisecond)
		}
	case "set-volume", "fade-to":
		if event.LevelDB != nil {
			p.setVolume(*event.LevelDB, time.Duration(event.FadeMs)*time.Millisecond, event.Curve)
		}
	case "mute":
		p.setMuted(true)
	case "unmute":
		p.setMuted(false)
	case "fade-out":
		p.setVolume(-80, time.Duration(event.FadeMs)*time.Millisecond, event.Curve)
	case "stop":
		if event.FadeMs > 0 {
			p.setVolume(-80, time.Duration(event.FadeMs)*time.Millisecond, event.Curve)
		} else {
			p.Close(true)
		}
	}
}

func (p *Player) pause() {
	p.mu.Lock()
	if p.closed || p.paused {
		p.mu.Unlock()
		return
	}
	p.position += time.Since(p.startedAt)
	p.paused = true
	p.generation++
	p.stopCommandsLocked()
	p.mu.Unlock()
}

func (p *Player) resume() {
	p.mu.RLock()
	position, paused := p.position, p.paused
	p.mu.RUnlock()
	if paused {
		_ = p.restart(position)
	}
}

func (p *Player) setMuted(muted bool) {
	p.mu.Lock()
	p.muted = muted
	if p.audio != nil {
		p.audio.SetVolume(dbVolume(p.volumeDB, p.muted))
	}
	p.mu.Unlock()
}

func (p *Player) setVolume(target float64, duration time.Duration, curve show.FadeCurve) {
	p.mu.Lock()
	start := p.volumeDB
	p.volumeFadeID++
	fadeID := p.volumeFadeID
	p.mu.Unlock()
	if duration <= 0 {
		p.applyVolume(target)
		return
	}
	go func() {
		started := time.Now()
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for now := range ticker.C {
			progress := min(1.0, float64(now.Sub(started))/float64(duration))
			volumeDB := fadeVolumeDB(start, target, progress, curve)
			if !p.applyFadeVolume(volumeDB, fadeID) {
				return
			}
			if progress >= 1 {
				return
			}
		}
	}()
}

func fadeVolumeDB(startDB, targetDB, progress float64, curve show.FadeCurve) float64 {
	progress = min(1.0, max(0.0, progress))
	if progress <= 0 {
		return startDB
	}
	if progress >= 1 {
		return targetDB
	}

	startGain, targetGain := dbVolume(startDB, false), dbVolume(targetDB, false)
	if curve == show.FadeCurveEqualPower {
		if targetGain < startGain {
			progress = 1 - math.Cos(progress*math.Pi/2)
		} else {
			progress = math.Sin(progress * math.Pi / 2)
		}
	}
	gain := startGain + (targetGain-startGain)*progress
	if gain <= 0 {
		return -80
	}
	return max(-80.0, 20*math.Log10(gain))
}

func (p *Player) applyFadeVolume(db float64, fadeID uint64) bool {
	p.mu.Lock()
	if p.closed || p.volumeFadeID != fadeID {
		p.mu.Unlock()
		return false
	}
	p.volumeDB = db
	if p.audio != nil {
		p.audio.SetVolume(dbVolume(db, p.muted))
	}
	p.mu.Unlock()
	p.window.Invalidate()
	return true
}

func (p *Player) applyVolume(db float64) {
	p.mu.Lock()
	p.volumeFadeID++
	p.volumeDB = db
	if p.audio != nil {
		p.audio.SetVolume(dbVolume(db, p.muted))
	}
	p.mu.Unlock()
	p.window.Invalidate()
}

func (p *Player) Layout(gtx layout.Context) layout.Dimensions {
	p.mu.RLock()
	frame, started, fadeIn, volume := p.frame, p.started, p.instance.FadeInMs, p.volumeDB
	p.mu.RUnlock()
	if frame == nil {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	opacity := float32(1)
	if fadeIn > 0 {
		opacity = float32(min(1.0, float64(time.Since(started))/float64(time.Duration(fadeIn)*time.Millisecond)))
		if opacity < 1 {
			gtx.Execute(op.InvalidateCmd{At: time.Now().Add(time.Second / 60)})
		}
	}
	if volume < 0 {
		opacity *= float32(dbVolume(volume, false))
	}
	stack := paint.PushOpacity(gtx.Ops, opacity)
	defer stack.Pop()
	return widget.Image{Src: paint.NewImageOp(frame), Fit: widget.Contain, Position: layout.Center}.Layout(gtx)
}

func (p *Player) Close(report bool) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.generation++
	p.stopCommandsLocked()
	p.mu.Unlock()
	if report {
		p.report("stopped")
	}
}

func (p *Player) stopCommandsLocked() {
	// Stop FFmpeg before uninitializing the device so a device callback blocked
	// on the decoder pipe is released before the audio backend waits for it.
	if p.audioCommand != nil && p.audioCommand.Process != nil {
		_ = p.audioCommand.Process.Kill()
		p.audioCommand = nil
	}
	if p.audio != nil {
		_ = p.audio.Close()
		p.audio = nil
	}
	if p.videoCommand != nil && p.videoCommand.Process != nil {
		_ = p.videoCommand.Process.Kill()
		p.videoCommand = nil
	}
}

func sourcePath(source string) (string, error) {
	if strings.HasPrefix(source, "file:") {
		parsed, err := url.Parse(source)
		if err != nil {
			return "", err
		}
		source = parsed.Path
		if runtime.GOOS == "windows" && len(source) >= 3 && source[0] == '/' && source[2] == ':' {
			source = source[1:]
		}
	}
	source = filepath.FromSlash(source)
	if !filepath.IsAbs(source) {
		absolute, err := filepath.Abs(source)
		if err != nil {
			return "", err
		}
		source = absolute
	}
	return source, nil
}

func probeVideo(ffmpegPath, source string) (int, int, error) {
	probe := ffprobePath(ffmpegPath)
	command := exec.Command(probe, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=width,height", "-of", "json", source)
	raw, err := command.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("probe video: %w", err)
	}
	var result struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(raw, &result); err != nil || len(result.Streams) == 0 || result.Streams[0].Width <= 0 || result.Streams[0].Height <= 0 {
		return 0, 0, errors.New("video has no decodable video stream")
	}
	return result.Streams[0].Width, result.Streams[0].Height, nil
}

func probeMediaDuration(ffmpegPath, source string) (time.Duration, error) {
	command := exec.Command(ffprobePath(ffmpegPath), "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", source)
	output, err := command.Output()
	if err != nil {
		return 0, fmt.Errorf("probe media duration: %w", err)
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil || seconds <= 0 {
		return 0, fmt.Errorf("invalid media duration %q", strings.TrimSpace(string(output)))
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

// ProbeDurationMs reads a media file's full duration without starting
// playback. Source may be a normal path or a file URI.
func ProbeDurationMs(ffmpegPath, source string) (int64, error) {
	path, err := sourcePath(source)
	if err != nil {
		return 0, err
	}
	duration, err := probeMediaDuration(ffmpegPath, path)
	if err != nil {
		return 0, err
	}
	return duration.Milliseconds(), nil
}

func ffprobePath(ffmpegPath string) string {
	if filepath.IsAbs(ffmpegPath) {
		return filepath.Join(filepath.Dir(ffmpegPath), "ffprobe"+filepath.Ext(ffmpegPath))
	}
	return "ffprobe"
}

func dbVolume(db float64, muted bool) float64 {
	if muted || db <= -80 {
		return 0
	}
	return min(1.0, math.Pow(10, db/20))
}
