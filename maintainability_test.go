package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	maximumProductionFileLines = 700
	maximumFormerLargeFileLines = 500
)

var formerLargeFiles = map[string]struct{}{
	"main.go":                    {},
	"media/audio.go":             {},
	"media/backend.go":           {},
	"media/manager.go":           {},
	"media/player.go":            {},
	"playback/engine.go":         {},
	"project/archive.go":         {},
	"show/manager.go":            {},
	"show/warnings.go":           {},
	"ui/cue_edit_pages.go":       {},
	"ui/main_page.go":            {},
	"ui/operator_panel.go":       {},
	"ui/settings_page.go":        {},
	"ui/tb_context.go":           {},
	"ui/timecode_timeline.go":    {},
}

func TestProductionFilesStayWithinMaintainabilityLimits(t *testing.T) {
	t.Parallel()

	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != "." && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		normalized := filepath.ToSlash(path)
		normalized = strings.TrimPrefix(normalized, "./")
		limit := maximumProductionFileLines
		if _, wasOversized := formerLargeFiles[normalized]; wasOversized {
			limit = maximumFormerLargeFileLines
		}
		lines, err := countProductionLines(path)
		if err != nil {
			return err
		}
		if lines > limit {
			t.Errorf("%s has %d lines; limit is %d", normalized, lines, limit)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func countProductionLines(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	lines := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		lines++
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("scan %s: %w", path, err)
	}
	return lines, nil
}
