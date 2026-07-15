package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gioui.org/x/explorer"

	"github.com/syspoe/cusus/project"
)

func closeWithError(closer io.Closer, current error) error {
	if closeErr := closer.Close(); current == nil {
		return closeErr
	}
	return current
}

func formatFileCount(count int) string {
	if count == 1 {
		return "1 media file"
	}
	return fmt.Sprintf("%d media files", count)
}

func formatSaveProgress(path string, progress project.SaveProgress) string {
	return fmt.Sprintf("Saving %s · bundling %s %d/%d · %s", documentName(path), progress.Kind, progress.Current, progress.Total, progress.Name)
}

func explorerPath(file any) string {
	var source string
	switch file := file.(type) {
	case *explorer.File:
		source = file.URI()
	case *os.File:
		source = file.Name()
	}
	path, err := project.LocalPath(source)
	if err != nil {
		return ""
	}
	return path
}

func documentName(path string) string {
	if strings.TrimSpace(path) == "" {
		return "show.cusus"
	}
	return filepath.Base(path)
}
