package operatorlog

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const supportFileLimit int64 = 5 << 20

type supportManifest struct {
	SchemaVersion int       `json:"schemaVersion"`
	CreatedAt     time.Time `json:"createdAt"`
	SessionID     string    `json:"sessionId"`
	BuildID       string    `json:"buildId"`
	ShowID        string    `json:"showId,omitempty"`
	EventCount    int       `json:"eventCount"`
}

// ExportSupportBundle writes bounded operational logs, a redacted settings
// snapshot, recent crash reports, and an identity manifest to a portable zip.
func (s *Store) ExportSupportBundle(destination, settingsPath, crashDirectory string) error {
	destination = filepath.Clean(destination)
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".support-*.zip")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()

	archive := zip.NewWriter(temporary)
	s.mu.RLock()
	manifest := supportManifest{SchemaVersion: 1, CreatedAt: time.Now(), SessionID: s.sessionID, BuildID: s.buildID, EventCount: len(s.events)}
	if s.showID != nil {
		manifest.ShowID = s.showID()
	}
	logPath := s.logPath
	events := append([]Event(nil), s.events...)
	s.mu.RUnlock()

	if err := writeSupportJSON(archive, "manifest.json", manifest); err != nil {
		return err
	}
	if err := writeSupportJSON(archive, "events/current-session.json", events); err != nil {
		return err
	}
	if settingsPath != "" {
		if err := addRedactedSettings(archive, settingsPath); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	for generation := 0; generation <= 4; generation++ {
		path := logPath
		if generation > 0 {
			path += fmt.Sprintf(".%d", generation)
		}
		if err := addBoundedSupportFile(archive, fmt.Sprintf("logs/operator-events.%d.jsonl", generation), path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := addRecentCrashReports(archive, crashDirectory, 5); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := archive.Close(); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return err
	}
	committed = true
	return nil
}

func writeSupportJSON(archive *zip.Writer, name string, value any) error {
	entry, err := archive.Create(name)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(entry)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func addRedactedSettings(archive *zip.Writer, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("read settings for support bundle: %w", err)
	}
	redactSupportValue(value)
	return writeSupportJSON(archive, "configuration/settings.redacted.json", value)
}

func redactSupportValue(value any) {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "credential") || strings.HasSuffix(lower, "key") {
				value[key] = "[REDACTED]"
				continue
			}
			redactSupportValue(child)
		}
	case []any:
		for _, child := range value {
			redactSupportValue(child)
		}
	}
}

func addBoundedSupportFile(archive *zip.Writer, name, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	entry, err := archive.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(entry, io.LimitReader(file, supportFileLimit))
	return err
}

func addRecentCrashReports(archive *zip.Writer, directory string, limit int) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool {
		left, _ := entries[i].Info()
		right, _ := entries[j].Info()
		return left.ModTime().After(right.ModTime())
	})
	added := 0
	for _, entry := range entries {
		if entry.IsDir() || added >= limit {
			continue
		}
		if err := addBoundedSupportFile(archive, "crashes/"+filepath.Base(entry.Name()), filepath.Join(directory, entry.Name())); err != nil {
			return err
		}
		added++
	}
	return nil
}
