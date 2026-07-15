package media

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strconv"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/internal/processgroup"
)

type audioRecoveryRequest struct {
	ctx        context.Context
	path       string
	position   time.Duration
	clipEndMs  int64
	preview    bool
	volumeDB   float64
	onRecovery func(string) error
}

type recoveredAudioEndpoint struct {
	command *exec.Cmd
	player  *devicePlayer
	stderr  *bytes.Buffer
}

func (endpoint *recoveredAudioEndpoint) close() {
	if endpoint == nil {
		return
	}
	if endpoint.player != nil {
		_ = endpoint.player.Close()
	}
	if endpoint.command != nil && endpoint.command.Process != nil {
		_ = endpoint.command.Process.Kill()
		_ = endpoint.command.Wait()
	}
}

func (endpoint *recoveredAudioEndpoint) bindClock(clock *PlaybackClock) {
	if endpoint != nil && endpoint.player != nil && clock != nil {
		clock.SetMaster(endpoint.player.RenderedPosition)
	}
}

type audioEndpointRecovery interface {
	recover(string, audioRecoveryRequest) (*recoveredAudioEndpoint, error)
}

// ffmpegAudioEndpointRecovery owns the route-level replacement operation:
// respawn a seeked PCM decoder, prepare and start its device endpoint, then
// return the pair for an atomic session swap.
type ffmpegAudioEndpointRecovery struct {
	settings *config.Store
	audio    *AudioSystem
}

func newFFmpegAudioEndpointRecovery(settings *config.Store, audio *AudioSystem) audioEndpointRecovery {
	return &ffmpegAudioEndpointRecovery{settings: settings, audio: audio}
}

func (recovery *ffmpegAudioEndpointRecovery) recover(targetDeviceID string, request audioRecoveryRequest) (*recoveredAudioEndpoint, error) {
	if recovery == nil || recovery.settings == nil || recovery.audio == nil {
		return nil, errors.New("audio endpoint recovery is unavailable")
	}
	settings := recovery.settings.Snapshot()
	cmd := processgroup.CommandContext(request.ctx, settings.FFmpegPath, pcmDecodeArgs(request.position, request.clipEndMs, request.path)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr
	if err := processgroup.Start(cmd); err != nil {
		return nil, err
	}
	reader := bufio.NewReaderSize(stdout, 64*1024)
	if _, err := reader.Peek(4096); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, ffmpegCommandError("recover audio", err, stderr.String())
	}
	_, policy, backupID := config.AudioRoute(settings, request.preview)
	player, err := recovery.audio.newPreparedPlayer(reader, targetDeviceID, policy, backupID)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}
	player.SetRecoveryHandler(request.onRecovery)
	player.SetVolume(request.volumeDB)
	if err := player.Start(); err != nil {
		_ = player.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}
	return &recoveredAudioEndpoint{command: cmd, player: player, stderr: stderr}, nil
}

func pcmDecodeArgs(position time.Duration, clipEndMs int64, path string) []string {
	args := mediaInputArgs(position, clipEndMs)
	return append(args, "-i", path, "-map", "0:a:0", "-vn", "-f", "s16le", "-ar", strconv.Itoa(audioSampleRate), "-ac", strconv.Itoa(audioChannels), "pipe:1")
}

// recoverAudio coordinates generation ownership while the recovery collaborator
// performs all FFmpeg respawn and device-route work.
func (s *ffmpegSession) recoverAudio(targetDeviceID string) error {
	s.mu.Lock()
	if s.closed || s.clock == nil || s.audio.player == nil || s.backend == nil || s.backend.recovery == nil {
		s.mu.Unlock()
		return errors.New("media session is not available for audio recovery")
	}
	position := s.clock.Position()
	oldCommand, oldPlayer := s.audio.command, s.audio.player
	s.audio.generation++
	generation := s.audio.generation
	request := audioRecoveryRequest{
		ctx: s.ctx, path: s.path, position: position, clipEndMs: s.request.Instance.ClipEndMs,
		preview: s.request.Instance.Preview, volumeDB: dbVolume(s.volume, s.muted), onRecovery: s.recoverAudio,
	}
	recovery := s.backend.recovery
	clock := s.clock
	s.component.Add(1)
	s.mu.Unlock()
	if oldCommand != nil && oldCommand.Process != nil {
		_ = oldCommand.Process.Kill()
	}

	replacement, err := recovery.recover(targetDeviceID, request)
	if err != nil {
		s.component.Done()
		return err
	}
	if replacement == nil || replacement.command == nil || replacement.player == nil || replacement.stderr == nil {
		if replacement != nil {
			replacement.close()
		}
		s.component.Done()
		return errors.New("audio endpoint recovery returned an incomplete replacement")
	}
	s.mu.Lock()
	if s.closed || s.audio.generation != generation {
		s.mu.Unlock()
		replacement.close()
		s.component.Done()
		return errors.New("audio recovery was superseded")
	}
	s.audio.command, s.audio.player = replacement.command, replacement.player
	replacement.bindClock(clock)
	s.mu.Unlock()
	_ = oldPlayer.Close()
	go s.waitAudioCommand(replacement.command, replacement.stderr, generation)
	go s.watchAudioDevice(replacement.player)
	return nil
}
