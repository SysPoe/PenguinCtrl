package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/syspoe/cusus/internal/processgroup"
	_ "golang.org/x/image/webp"
)

const (
	probeTimeout               = 10 * time.Second
	defaultVideoFPS            = 30.0
	rotationQuarterTurnEpsilon = 0.01
	maxVideoDimension          = 8192
	maxVideoPixels             = 7680 * 4320
	maxVideoFrameRate          = 240
	maxVideoBitRate            = 500_000_000
	maxAudioBitRate            = 20_000_000
)

// Kind describes the stream capability required from a source. It deliberately
// does not depend on show.CueType so offline analysis can be reused without the
// playback or show model layers.
type Kind uint8

const (
	KindAudio Kind = iota
	KindVideo
	KindImage
)

// Info is the bounded stream metadata used to prepare a decoder.
type Info struct {
	Width, Height int
	FPS           float64
	VideoBitRate  int64
	AudioBitRate  int64
	HasVideo      bool
	HasAudio      bool
}

func (i Info) FrameInterval() time.Duration {
	if i.FPS <= 0 {
		return time.Second / time.Duration(defaultVideoFPS)
	}
	return time.Duration(float64(time.Second) / i.FPS)
}

// ProbeStreams resolves source and reads its bounded audio/video metadata.
func ProbeStreams(parent context.Context, ffmpegPath, source string) (Info, error) {
	path, err := ResolveSource(source)
	if err != nil {
		return Info{}, err
	}
	ctx, cancel := context.WithTimeout(parent, probeTimeout)
	defer cancel()
	command := processgroup.CommandContext(ctx, ffprobeExecutable(ffmpegPath), "-v", "error", "-show_entries", "stream=codec_type,width,height,avg_frame_rate,bit_rate:stream_tags=rotate:stream_side_data=rotation", "-of", "json", path)
	raw, err := processgroup.CombinedOutput(command)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Info{}, fmt.Errorf("ffmpeg probe media streams timed out after %s", probeTimeout)
		}
		return Info{}, commandError("probe media streams", err, string(raw))
	}
	return parseInfo(raw)
}

// ProbeDuration returns the duration of source after resolving its path.
func ProbeDuration(parent context.Context, ffmpegPath, source string) (time.Duration, error) {
	path, err := ResolveSource(source)
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(parent, probeTimeout)
	defer cancel()
	command := processgroup.CommandContext(ctx, ffprobeExecutable(ffmpegPath), "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path)
	output, err := processgroup.Output(command)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return 0, fmt.Errorf("probe media duration timed out after %s", probeTimeout)
		}
		return 0, fmt.Errorf("probe media duration: %w", err)
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil || seconds <= 0 {
		return 0, fmt.Errorf("invalid media duration %q", strings.TrimSpace(string(output)))
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

// Validate checks that source is readable as kind and satisfies decoder
// resource limits.
func Validate(parent context.Context, ffmpegPath, source string, kind Kind) error {
	if kind == KindImage {
		path, err := ResolveSource(source)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open image: %w", err)
		}
		defer file.Close()
		if _, _, err := image.DecodeConfig(file); err != nil {
			return fmt.Errorf("unsupported image: %w", err)
		}
		return nil
	}
	info, err := ProbeStreams(parent, ffmpegPath, source)
	if err != nil {
		return err
	}
	if kind == KindAudio && !info.HasAudio {
		return errors.New("audio stream could not be opened: file has no audio stream")
	}
	if kind == KindVideo && !info.HasVideo {
		return errors.New("video stream could not be opened: file has no video stream")
	}
	return nil
}

func parseInfo(raw []byte) (Info, error) {
	var result struct {
		Streams []struct {
			CodecType    string `json:"codec_type"`
			Width        int    `json:"width"`
			Height       int    `json:"height"`
			AvgFrameRate string `json:"avg_frame_rate"`
			BitRate      string `json:"bit_rate"`
			Tags         struct {
				Rotate string `json:"rotate"`
			} `json:"tags"`
			SideData []struct {
				Rotation *float64 `json:"rotation"`
			} `json:"side_data_list"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return Info{}, err
	}
	var info Info
	for _, stream := range result.Streams {
		switch stream.CodecType {
		case "video":
			if !info.HasVideo {
				info.HasVideo, info.Width, info.Height = true, stream.Width, stream.Height
				info.FPS = parseFrameRate(stream.AvgFrameRate)
				info.VideoBitRate, _ = strconv.ParseInt(stream.BitRate, 10, 64)
				rotation, _ := strconv.ParseFloat(stream.Tags.Rotate, 64)
				for _, sideData := range stream.SideData {
					if sideData.Rotation != nil {
						rotation = *sideData.Rotation
						break
					}
				}
				quarterTurns := math.Round(rotation / 90)
				if math.Abs(rotation-quarterTurns*90) < rotationQuarterTurnEpsilon && int(quarterTurns)%2 != 0 {
					info.Width, info.Height = info.Height, info.Width
				}
			}
		case "audio":
			info.HasAudio = true
			bitRate, _ := strconv.ParseInt(stream.BitRate, 10, 64)
			info.AudioBitRate += max(int64(0), bitRate)
		}
	}
	if info.HasVideo {
		if info.Width <= 0 || info.Height <= 0 || info.Width > maxVideoDimension || info.Height > maxVideoDimension || int64(info.Width)*int64(info.Height) > maxVideoPixels {
			return Info{}, fmt.Errorf("video dimensions %dx%d exceed the supported decode limit", info.Width, info.Height)
		}
		if info.FPS > maxVideoFrameRate {
			return Info{}, fmt.Errorf("video frame rate %.2f exceeds the supported %.0f fps limit", info.FPS, float64(maxVideoFrameRate))
		}
		if info.VideoBitRate > maxVideoBitRate {
			return Info{}, fmt.Errorf("video bitrate %d exceeds the supported %d bit/s limit", info.VideoBitRate, maxVideoBitRate)
		}
	}
	if info.AudioBitRate > maxAudioBitRate {
		return Info{}, fmt.Errorf("audio bitrate %d exceeds the supported %d bit/s limit", info.AudioBitRate, maxAudioBitRate)
	}
	return info, nil
}

func parseFrameRate(value string) float64 {
	parts := strings.Split(value, "/")
	if len(parts) == 2 {
		numerator, errN := strconv.ParseFloat(parts[0], 64)
		denominator, errD := strconv.ParseFloat(parts[1], 64)
		if errN == nil && errD == nil && denominator > 0 {
			return numerator / denominator
		}
	}
	fps, _ := strconv.ParseFloat(value, 64)
	return fps
}

func commandError(operation string, err error, stderr string) error {
	detail := strings.TrimSpace(stderr)
	if detail == "" {
		return fmt.Errorf("ffmpeg %s failed: %w", operation, err)
	}
	return fmt.Errorf("ffmpeg %s failed: %w: %s", operation, err, detail)
}
