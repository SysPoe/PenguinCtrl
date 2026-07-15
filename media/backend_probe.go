package media

import (
	"context"
	"fmt"
	"strings"

	mediaanalysis "github.com/syspoe/cusus/media/analysis"
	"github.com/syspoe/cusus/show"
)

type mediaInfo = mediaanalysis.Info

func probeMediaInfo(ffmpegPath, source string) (mediaInfo, error) {
	return mediaanalysis.ProbeStreams(context.Background(), ffmpegPath, source)
}

func ffmpegCommandError(operation string, err error, stderr string) error {
	detail := strings.TrimSpace(stderr)
	if detail == "" {
		return fmt.Errorf("ffmpeg %s failed: %w", operation, err)
	}
	return fmt.Errorf("ffmpeg %s failed: %w: %s", operation, err, detail)
}

// ValidateSource preserves the application-facing cue validator while mapping
// the show model to the analysis package's independent media kind.
func ValidateSource(ffmpegPath, source string, cueType show.CueType) error {
	kind := mediaanalysis.KindAudio
	switch cueType {
	case show.CueTypeVideo:
		kind = mediaanalysis.KindVideo
	case show.CueTypeImage:
		kind = mediaanalysis.KindImage
	}
	return mediaanalysis.Validate(context.Background(), ffmpegPath, source, kind)
}
