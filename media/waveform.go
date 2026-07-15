package media

import (
	"context"

	mediaanalysis "github.com/syspoe/cusus/media/analysis"
)

// Waveform remains an alias for compatibility with application callbacks.
type Waveform = mediaanalysis.Waveform

func ExtractWaveform(ffmpegPath, source string) (Waveform, error) {
	return mediaanalysis.ExtractWaveform(context.Background(), ffmpegPath, source)
}

func ExtractWaveformContext(ctx context.Context, ffmpegPath, source string) (Waveform, error) {
	return mediaanalysis.ExtractWaveform(ctx, ffmpegPath, source)
}
