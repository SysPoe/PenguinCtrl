package media

import (
	"context"

	mediaanalysis "github.com/syspoe/cusus/media/analysis"
)

func sourcePath(source string) (string, error) {
	return mediaanalysis.ResolveSource(source)
}

func ProbeDurationMs(ffmpegPath, source string) (int64, error) {
	return ProbeDurationMsContext(context.Background(), ffmpegPath, source)
}

func ProbeDurationMsContext(ctx context.Context, ffmpegPath, source string) (int64, error) {
	duration, err := mediaanalysis.ProbeDuration(ctx, ffmpegPath, source)
	if err != nil {
		return 0, err
	}
	return duration.Milliseconds(), nil
}
