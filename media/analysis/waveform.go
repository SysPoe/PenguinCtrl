package analysis

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/syspoe/cusus/internal/processgroup"
)

const (
	waveformSampleRate    = 400
	waveformDecodeTimeout = 2 * time.Minute
	int16FullScale        = 32768.0
)

// Waveform is a low-rate mono representation of a media file. Samples are
// normalized peak magnitudes and are intentionally inexpensive to retain.
type Waveform struct {
	Samples    []float32
	SampleRate int
	DurationMs int64
}

// ExtractWaveform resolves source, probes its duration, then decodes the first
// audio stream. If decoding fails after the probe succeeds, the returned
// metadata remains populated while Samples is empty.
func ExtractWaveform(parent context.Context, ffmpegPath, source string) (Waveform, error) {
	path, err := ResolveSource(source)
	if err != nil {
		return Waveform{}, err
	}
	duration, err := ProbeDuration(parent, ffmpegPath, path)
	if err != nil {
		return Waveform{}, err
	}
	metadata := Waveform{SampleRate: waveformSampleRate, DurationMs: duration.Milliseconds()}
	ctx, cancel := context.WithTimeout(parent, waveformDecodeTimeout)
	defer cancel()
	cmd := processgroup.CommandContext(ctx, ffmpegPath, "-v", "error", "-i", path, "-map", "0:a:0", "-ac", "1", "-ar", strconv.Itoa(waveformSampleRate), "-f", "s16le", "pipe:1")
	raw, err := processgroup.Output(cmd)
	if err != nil {
		if ctx.Err() != nil {
			return metadata, fmt.Errorf("decode waveform timed out: %w", ctx.Err())
		}
		return metadata, fmt.Errorf("decode waveform: %w", err)
	}
	metadata.Samples = samplesFromPCM(raw)
	return metadata, nil
}

func samplesFromPCM(raw []byte) []float32 {
	samples := make([]float32, len(raw)/2)
	for i := range samples {
		value := int16(binary.LittleEndian.Uint16(raw[i*2:]))
		samples[i] = float32(math.Abs(float64(value)) / int16FullScale)
	}
	return samples
}
