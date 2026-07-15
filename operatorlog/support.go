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
// TODO(macro): Support-bundle assembly (settings redaction, crash-dir layout,
// zip packaging) is a diagnostics/export feature bolted onto Store. Extract a
// support/diagnostics package that takes a Store snapshot plus explicit
// settings/crash inputs so operatorlog does not own foreign file formats.
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
	events := append([]Event(nil), s.events...)
	s.mu.RUnlock()
	logPath := s.log.Path()

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
	for generation := 0; generation <= eventLogGenerations; generation++ {
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
	type crashReport struct {
		entry os.DirEntry
		info  os.FileInfo
	}
	reports := make([]crashReport, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "crash-") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect crash report %q: %w", entry.Name(), err)
		}
		reports = append(reports, crashReport{entry: entry, info: info})
	}
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].info.ModTime().After(reports[j].info.ModTime())
	})
	added := 0
	for _, report := range reports {
		if added >= limit {
			break
		}
		if err := addBoundedSupportFile(archive, "crashes/"+filepath.Base(report.entry.Name()), filepath.Join(directory, report.entry.Name())); err != nil {
			return err
		}
		added++
	}
	return nil
}
