// Package support assembles redacted diagnostic bundles for support handoff.
package support

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

	"github.com/syspoe/cusus/operatorlog"
)

const (
	supportFileLimit = int64(5 << 20)
	crashFileLimit   = 5
)

type manifest struct {
	SchemaVersion int       `json:"schemaVersion"`
	CreatedAt     time.Time `json:"createdAt"`
	SessionID     string    `json:"sessionId"`
	BuildID       string    `json:"buildId"`
	ShowID        string    `json:"showId,omitempty"`
	EventCount    int       `json:"eventCount"`
}

// Export writes bounded logs, redacted settings, recent crash reports, and an
// identity manifest to a portable ZIP archive.
func Export(destination string, snapshot operatorlog.DiagnosticSnapshot, settingsPath, crashDirectory string) error {
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
	metadata := manifest{SchemaVersion: 1, CreatedAt: time.Now(), SessionID: snapshot.SessionID, BuildID: snapshot.BuildID, ShowID: snapshot.ShowID, EventCount: len(snapshot.Events)}
	if err := writeJSON(archive, "manifest.json", metadata); err != nil {
		return err
	}
	if err := writeJSON(archive, "events/current-session.json", snapshot.Events); err != nil {
		return err
	}
	if settingsPath != "" {
		if err := addRedactedSettings(archive, settingsPath); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	for index, path := range snapshot.LogPaths {
		if err := addBoundedFile(archive, fmt.Sprintf("logs/operator-events.%d.jsonl", index), path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := addRecentCrashReports(archive, crashDirectory, crashFileLimit); err != nil && !os.IsNotExist(err) {
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

func writeJSON(archive *zip.Writer, name string, value any) error {
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
	redact(value)
	return writeJSON(archive, "configuration/settings.redacted.json", value)
}

func redact(value any) {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "credential") || strings.HasSuffix(lower, "key") {
				value[key] = "[REDACTED]"
				continue
			}
			redact(child)
		}
	case []any:
		for _, child := range value {
			redact(child)
		}
	}
}

func addBoundedFile(archive *zip.Writer, name, path string) error {
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
	sort.Slice(reports, func(i, j int) bool { return reports[i].info.ModTime().After(reports[j].info.ModTime()) })
	for index, report := range reports {
		if index >= limit {
			break
		}
		if err := addBoundedFile(archive, "crashes/"+filepath.Base(report.entry.Name()), filepath.Join(directory, report.entry.Name())); err != nil {
			return err
		}
	}
	return nil
}
