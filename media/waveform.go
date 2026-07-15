package media

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"github.com/syspoe/cusus/internal/processgroup"
)

// Waveform is a low-rate mono representation of a media file. Samples are
// normalized peak magnitudes and are intentionally inexpensive to retain and
// redraw in the cue editor.
// TODO(macro): Offline editor analysis (waveform extract + duration probe) is
// co-located with the live playback runtime (windows, mixers, decoders). Move
// extract/probe helpers to a media-analysis surface that depends only on FFmpeg
// path utilities so the playback package's import graph and lifecycle stay free
// of editor tooling.
type Waveform struct {
	Samples    []float32
	SampleRate int
	DurationMs int64
}

// ExtractWaveform decodes the first audio stream with FFmpeg. The low sample
// rate is sufficient for a visual overview while keeping long show files
// responsive in the native editor.
func ExtractWaveform(ffmpegPath, source string) (Waveform, error) {
	return ExtractWaveformContext(context.Background(), ffmpegPath, source)
}

func ExtractWaveformContext(parent context.Context, ffmpegPath, source string) (Waveform, error) {
	path, err := sourcePath(source)
	if err != nil {
		return Waveform{}, err
	}
	duration, err := probeMediaDuration(ffmpegPath, path)
	if err != nil {
		return Waveform{}, err
	}
	// TODO(micro): 400 Hz overview rate and 2-minute decode timeout are magic; promote to named package constants.
	const sampleRate = 400
	// TODO(micro): timeout error path still returns SampleRate/DurationMs while other errors do too inconsistently with empty Samples; decide one partial-result policy.
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	// TODO(micro): Use strconv.Itoa for sampleRate instead of the reflection-backed fmt.Sprint path.
	cmd := processgroup.CommandContext(ctx, ffmpegPath, "-v", "error", "-i", path, "-map", "0:a:0", "-ac", "1", "-ar", fmt.Sprint(sampleRate), "-f", "s16le", "pipe:1")
	raw, err := processgroup.Output(cmd)
	if err != nil {
		if ctx.Err() != nil {
			return Waveform{SampleRate: sampleRate, DurationMs: duration.Milliseconds()}, fmt.Errorf("decode waveform timed out: %w", ctx.Err())
		}
		return Waveform{SampleRate: sampleRate, DurationMs: duration.Milliseconds()}, fmt.Errorf("decode waveform: %w", err)
	}
	samples := make([]float32, len(raw)/2)
	for i := range samples {
		value := int16(binary.LittleEndian.Uint16(raw[i*2:]))
		// TODO(micro): 32768 full-scale divisor is magic; name int16FullScale and use 32768.0 for readability.
		samples[i] = float32(math.Abs(float64(value)) / 32768)
	}
	return Waveform{Samples: samples, SampleRate: sampleRate, DurationMs: duration.Milliseconds()}, nil
}
