package media

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
	"github.com/syspoe/cusus/show"
	_ "golang.org/x/image/webp"
)

func ffmpegCommandError(operation string, err error, stderr string) error {
	detail := strings.TrimSpace(stderr)
	if detail == "" {
		return fmt.Errorf("ffmpeg %s failed: %w", operation, err)
	}
	return fmt.Errorf("ffmpeg %s failed: %w: %s", operation, err, detail)
}

type mediaInfo struct {
	width, height int
	fps           float64
	videoBitRate  int64
	audioBitRate  int64
	hasVideo      bool
	hasAudio      bool
}

func (i mediaInfo) frameInterval() time.Duration {
	if i.fps <= 0 {
		return time.Second / 30
	}
	return time.Duration(float64(time.Second) / i.fps)
}

func probeMediaInfo(ffmpegPath, source string) (mediaInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), mediaProbeTimeout)
	defer cancel()
	command := processgroup.CommandContext(ctx, ffprobePath(ffmpegPath), "-v", "error", "-show_entries", "stream=codec_type,width,height,avg_frame_rate,bit_rate:stream_tags=rotate:stream_side_data=rotation", "-of", "json", source)
	raw, err := processgroup.CombinedOutput(command)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return mediaInfo{}, fmt.Errorf("ffmpeg probe media streams timed out after %s", mediaProbeTimeout)
		}
		return mediaInfo{}, ffmpegCommandError("probe media streams", err, string(raw))
	}
	return parseMediaInfo(raw)
}

func parseMediaInfo(raw []byte) (mediaInfo, error) {
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
		return mediaInfo{}, err
	}
	var info mediaInfo
	for _, stream := range result.Streams {
		switch stream.CodecType {
		case "video":
			if !info.hasVideo {
				info.hasVideo, info.width, info.height = true, stream.Width, stream.Height
				info.fps = parseFrameRate(stream.AvgFrameRate)
				info.videoBitRate, _ = strconv.ParseInt(stream.BitRate, 10, 64)
				rotation, _ := strconv.ParseFloat(stream.Tags.Rotate, 64)
				for _, sideData := range stream.SideData {
					if sideData.Rotation != nil {
						rotation = *sideData.Rotation
						break
					}
				}
				// FFmpeg applies display rotation while decoding. Keep the raw
				// frame dimensions in step with that output or each frame is
				// interpreted with the wrong row stride.
				quarterTurns := math.Round(rotation / 90)
				if math.Abs(rotation-quarterTurns*90) < 0.01 && int(quarterTurns)%2 != 0 {
					info.width, info.height = info.height, info.width
				}
			}
		case "audio":
			info.hasAudio = true
			bitRate, _ := strconv.ParseInt(stream.BitRate, 10, 64)
			info.audioBitRate += max(int64(0), bitRate)
		}
	}
	if info.hasVideo {
		if info.width <= 0 || info.height <= 0 || info.width > maxVideoDimension || info.height > maxVideoDimension || int64(info.width)*int64(info.height) > maxVideoPixels {
			return mediaInfo{}, fmt.Errorf("video dimensions %dx%d exceed the supported decode limit", info.width, info.height)
		}
		if info.fps > maxVideoFrameRate {
			return mediaInfo{}, fmt.Errorf("video frame rate %.2f exceeds the supported %.0f fps limit", info.fps, float64(maxVideoFrameRate))
		}
		if info.videoBitRate > maxVideoBitRate {
			return mediaInfo{}, fmt.Errorf("video bitrate %d exceeds the supported %d bit/s limit", info.videoBitRate, maxVideoBitRate)
		}
	}
	if info.audioBitRate > maxAudioBitRate {
		return mediaInfo{}, fmt.Errorf("audio bitrate %d exceeds the supported %d bit/s limit", info.audioBitRate, maxAudioBitRate)
	}
	return info, nil
}

func ValidateSource(ffmpegPath, source string, cueType show.CueType) error {
	if cueType == show.CueTypeImage {
		path, err := sourcePath(source)
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
	info, err := probeMediaInfo(ffmpegPath, source)
	if err != nil {
		return err
	}
	if cueType == show.CueTypeSound && !info.hasAudio {
		return errors.New("audio stream could not be opened: file has no audio stream")
	}
	if cueType == show.CueTypeVideo && !info.hasVideo {
		return errors.New("video stream could not be opened: file has no video stream")
	}
	return nil
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
