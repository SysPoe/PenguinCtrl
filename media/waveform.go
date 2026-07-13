package media

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os/exec"
	"time"
)

// Waveform is a low-rate mono representation of a media file. Samples are
// normalized peak magnitudes and are intentionally inexpensive to retain and
// redraw in the cue editor.
type Waveform struct {
	Samples    []float32
	SampleRate int
	DurationMs int64
}

// ExtractWaveform decodes the first audio stream with FFmpeg. The low sample
// rate is sufficient for a visual overview while keeping long show files
// responsive in the native editor.
func ExtractWaveform(ffmpegPath, source string) (Waveform, error) {
	path, err := sourcePath(source)
	if err != nil {
		return Waveform{}, err
	}
	duration, err := probeMediaDuration(ffmpegPath, path)
	if err != nil {
		return Waveform{}, err
	}
	const sampleRate = 400
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpegPath, "-v", "error", "-i", path, "-map", "0:a:0", "-ac", "1", "-ar", fmt.Sprint(sampleRate), "-f", "s16le", "pipe:1")
	raw, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return Waveform{SampleRate: sampleRate, DurationMs: duration.Milliseconds()}, fmt.Errorf("decode waveform timed out: %w", ctx.Err())
		}
		return Waveform{SampleRate: sampleRate, DurationMs: duration.Milliseconds()}, fmt.Errorf("decode waveform: %w", err)
	}
	samples := make([]float32, len(raw)/2)
	for i := range samples {
		value := int16(binary.LittleEndian.Uint16(raw[i*2:]))
		samples[i] = float32(math.Abs(float64(value)) / 32768)
	}
	return Waveform{Samples: samples, SampleRate: sampleRate, DurationMs: duration.Milliseconds()}, nil
}
