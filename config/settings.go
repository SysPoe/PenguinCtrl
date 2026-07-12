package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type RemoteTarget struct {
	Name    string `json:"name"`
	Host    string `json:"host"`
	OSCPort int    `json:"oscPort"`
	ERCPort int    `json:"ercPort"`
}

type VideoOutput struct {
	Stage            string `json:"stage"`
	DisplayID        string `json:"displayId,omitempty"`
	Fullscreen       bool   `json:"fullscreen"`
	X                int    `json:"x,omitempty"`
	Y                int    `json:"y,omitempty"`
	Width            int    `json:"width"`
	Height           int    `json:"height"`
	ResolutionWidth  int    `json:"resolutionWidth"`
	ResolutionHeight int    `json:"resolutionHeight"`
	Scaling          string `json:"scaling"`
	IdleBehavior     string `json:"idleBehavior"`
	TestGrid         bool   `json:"testGrid,omitempty"`
	SafeAreaPercent  int    `json:"safeAreaPercent,omitempty"`
	Layers           int    `json:"layers"`
}

type Settings struct {
	FFmpegPath          string            `json:"ffmpegPath"`
	DefaultPlayback     string            `json:"defaultPlayback"`
	DefaultMediaOutput  string            `json:"defaultMediaOutput"`
	PlaybackAudioDevice string            `json:"playbackAudioDevice,omitempty"`
	PreviewAudioDevice  string            `json:"previewAudioDevice,omitempty"`
	VideoOutputs        []VideoOutput     `json:"videoOutputs,omitempty"`
	Variables           map[string]string `json:"variables"`
	RemoteTargets       []RemoteTarget    `json:"remoteTargets"`
}

func Defaults() Settings {
	return Settings{
		FFmpegPath:         "ffmpeg",
		DefaultPlayback:    "1",
		DefaultMediaOutput: "main",
		VideoOutputs:       []VideoOutput{{Stage: "main", Fullscreen: true, Width: 960, Height: 540, ResolutionWidth: 1920, ResolutionHeight: 1080, Scaling: "contain", IdleBehavior: "black", Layers: 1}},
		Variables:          map[string]string{},
		RemoteTargets:      []RemoteTarget{{Name: "Local console", Host: "127.0.0.1", OSCPort: 8000, ERCPort: 6553}},
	}
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config directory: %w", err)
	}
	return filepath.Join(dir, "CuSus", "settings.json"), nil
}

type Store struct {
	mu       sync.RWMutex
	path     string
	settings Settings
}

func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		var err error
		path, err = DefaultPath()
		if err != nil {
			return nil, err
		}
	}
	store := &Store{path: path, settings: Defaults()}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := store.saveLocked(); err != nil {
			return nil, err
		}
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read settings: %w", err)
	}
	settings := Defaults()
	if err := json.Unmarshal(raw, &settings); err != nil {
		return nil, fmt.Errorf("decode settings: %w", err)
	}
	store.settings = normalize(settings)
	return store, nil
}

func (s *Store) Path() string { return s.path }

func (s *Store) Snapshot() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clone(s.settings)
}

func (s *Store) Update(settings Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for name := range settings.Variables {
		if !ValidVariableName(name) {
			return fmt.Errorf("invalid variable name %q", name)
		}
		if name == "defaultPlayback" || name == "defaultMediaOutput" || name == "cueNumber" {
			return fmt.Errorf("%s is a built-in variable", name)
		}
	}
	s.settings = normalize(settings)
	return s.saveLocked()
}

func (s *Store) UpdateVideoOutputGeometry(stage string, x, y, width, height int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.settings.VideoOutputs {
		output := &s.settings.VideoOutputs[i]
		if output.Stage == stage {
			output.X, output.Y = x, y
			if width > 0 && height > 0 {
				output.Width, output.Height = width, height
			}
			return s.saveLocked()
		}
	}
	return nil
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}
	raw, err := json.MarshalIndent(s.settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace settings: %w", err)
	}
	return nil
}

func normalize(in Settings) Settings {
	if strings.TrimSpace(in.FFmpegPath) == "" {
		in.FFmpegPath = "ffmpeg"
	}
	if strings.TrimSpace(in.DefaultPlayback) == "" {
		in.DefaultPlayback = "1"
	}
	if strings.TrimSpace(in.DefaultMediaOutput) == "" {
		in.DefaultMediaOutput = "main"
	}
	in.PlaybackAudioDevice = strings.TrimSpace(in.PlaybackAudioDevice)
	in.PreviewAudioDevice = strings.TrimSpace(in.PreviewAudioDevice)
	seenStages := make(map[string]struct{}, len(in.VideoOutputs))
	outputs := make([]VideoOutput, 0, len(in.VideoOutputs)+1)
	for _, output := range in.VideoOutputs {
		output.Stage, output.DisplayID = strings.TrimSpace(output.Stage), strings.TrimSpace(output.DisplayID)
		if output.Stage == "" {
			continue
		}
		if _, exists := seenStages[output.Stage]; exists {
			continue
		}
		seenStages[output.Stage] = struct{}{}
		if output.Width < 320 {
			output.Width = 960
		}
		if output.Height < 180 {
			output.Height = 540
		}
		if output.ResolutionWidth < 1 {
			output.ResolutionWidth = 1920
		}
		if output.ResolutionHeight < 1 {
			output.ResolutionHeight = 1080
		}
		switch output.Scaling {
		case "contain", "cover", "stretch", "native":
		default:
			output.Scaling = "contain"
		}
		if output.IdleBehavior != "hold" {
			output.IdleBehavior = "black"
		}
		output.SafeAreaPercent = min(20, max(0, output.SafeAreaPercent))
		output.Layers = min(8, max(1, output.Layers))
		outputs = append(outputs, output)
	}
	if _, exists := seenStages[in.DefaultMediaOutput]; !exists {
		outputs = append(outputs, VideoOutput{Stage: in.DefaultMediaOutput, Fullscreen: true, Width: 960, Height: 540, ResolutionWidth: 1920, ResolutionHeight: 1080, Scaling: "contain", IdleBehavior: "black", Layers: 1})
	}
	in.VideoOutputs = outputs
	if in.Variables == nil {
		in.Variables = map[string]string{}
	}
	for i := range in.RemoteTargets {
		target := &in.RemoteTargets[i]
		target.Name = strings.TrimSpace(target.Name)
		target.Host = strings.TrimSpace(target.Host)
		if target.Host == "" {
			target.Host = "127.0.0.1"
		}
		if target.OSCPort < 0 || target.OSCPort > 65535 {
			target.OSCPort = 0
		}
		if target.ERCPort < 0 || target.ERCPort > 65535 {
			target.ERCPort = 0
		}
	}
	return in
}

func clone(in Settings) Settings {
	out := in
	out.Variables = make(map[string]string, len(in.Variables))
	for key, value := range in.Variables {
		out.Variables[key] = value
	}
	out.RemoteTargets = append([]RemoteTarget(nil), in.RemoteTargets...)
	out.VideoOutputs = append([]VideoOutput(nil), in.VideoOutputs...)
	return out
}

func VideoOutputFor(settings Settings, stage string) VideoOutput {
	for _, output := range settings.VideoOutputs {
		if output.Stage == stage {
			return output
		}
	}
	fallback := Defaults().VideoOutputs[0]
	fallback.Stage = stage
	return fallback
}

var variableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)
var templatePattern = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_.-]*)([+-]\d+(?:\.\d{1,2})?)?\}`)

func ValidVariableName(name string) bool { return variableNamePattern.MatchString(name) }

func Resolve(template string, settings Settings, cueNumber string) string {
	values := make(map[string]string, len(settings.Variables)+3)
	for key, value := range settings.Variables {
		values[key] = value
	}
	values["defaultPlayback"] = settings.DefaultPlayback
	values["defaultMediaOutput"] = settings.DefaultMediaOutput
	values["cueNumber"] = cueNumber

	resolved := template
	for range 8 {
		next := templatePattern.ReplaceAllStringFunc(resolved, func(match string) string {
			parts := templatePattern.FindStringSubmatch(match)
			value, ok := values[parts[1]]
			if !ok {
				return match
			}
			if parts[1] == "cueNumber" && parts[2] != "" {
				if offsetValue, err := offsetCueNumber(value, parts[2]); err == nil {
					return offsetValue
				}
			}
			return value
		})
		if next == resolved {
			break
		}
		resolved = next
	}
	return resolved
}

func offsetCueNumber(base, offset string) (string, error) {
	baseValue, err := cueNumberToHundredths(base)
	if err != nil {
		return "", err
	}
	offsetValue, err := signedCueNumberToHundredths(offset)
	if err != nil {
		return "", err
	}
	value := max(0, baseValue+offsetValue)
	whole, fraction := value/100, value%100
	if fraction == 0 {
		return strconv.Itoa(whole), nil
	}
	return strings.TrimRight(fmt.Sprintf("%d.%02d", whole, fraction), "0"), nil
}

func cueNumberToHundredths(value string) (int, error) {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) > 2 || len(parts) == 0 || parts[0] == "" {
		return 0, fmt.Errorf("invalid cue number %q", value)
	}
	whole, err := strconv.Atoi(parts[0])
	if err != nil || whole < 0 {
		return 0, fmt.Errorf("invalid cue number %q", value)
	}
	fraction := 0
	if len(parts) == 2 {
		if len(parts[1]) == 0 || len(parts[1]) > 2 {
			return 0, fmt.Errorf("invalid cue number %q", value)
		}
		fraction, err = strconv.Atoi(parts[1] + strings.Repeat("0", 2-len(parts[1])))
		if err != nil {
			return 0, fmt.Errorf("invalid cue number %q", value)
		}
	}
	return whole*100 + fraction, nil
}

func signedCueNumberToHundredths(value string) (int, error) {
	if len(value) < 2 || (value[0] != '+' && value[0] != '-') {
		return 0, fmt.Errorf("invalid cue offset %q", value)
	}
	result, err := cueNumberToHundredths(value[1:])
	if err != nil {
		return 0, err
	}
	if value[0] == '-' {
		result = -result
	}
	return result, nil
}
