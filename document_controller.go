package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gioui.org/x/explorer"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/internal/taskgroup"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/playback"
	"github.com/syspoe/cusus/project"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/ui"
)

type documentExplorer interface {
	ChooseFile(...string) (io.ReadCloser, error)
	CreateFile(string) (io.WriteCloser, error)
}

type documentControllerConfig struct {
	explorer documentExplorer
	tasks    *taskgroup.Group
	postUI   func(context.Context, func()) bool
	manager  *show.ShowManager
	playback *playback.Engine
	library  *project.Library
	session  *documentSession
	journal  *project.EditJournal
	settings *config.Store
	events   *operatorlog.Store
	panel    *ui.OperatorPanel
}

type documentController struct{ documentControllerConfig }

const (
	documentOperationOpen = "Open"
	documentOperationSave = "Save"
)

func newDocumentController(config documentControllerConfig) *documentController {
	return &documentController{documentControllerConfig: config}
}

// bindShowChanges makes the document boundary responsible for recovery
// journaling while leaving unrelated runtime reactions with the composition
// root. ShowManager owns a single change callback, so this is bound once when
// the window session is assembled.
func (c *documentController) bindShowChanges(after func()) {
	c.manager.SetOnChange(func() {
		c.checkpointDirty()
		if after != nil {
			after()
		}
	})
}

func (c *documentController) checkpointDirty() {
	if c.journal == nil {
		return
	}
	current := c.manager.ShowSnapshot()
	path, dirty, suppressed := c.session.status(current)
	if suppressed || !dirty {
		return
	}
	if err := c.journal.RecordDirty(current, path); err != nil {
		c.events.Add(operatorlog.Recoverable, "Edit recovery", err.Error(), show.CueID{}, "")
	}
}

func (c *documentController) Load() {
	c.tasks.Go("open-show", func(ctx context.Context) {
		file, err := c.explorer.ChooseFile(".cusus")
		if err != nil {
			if !errors.Is(err, explorer.ErrUserDecline) {
				c.reportFailure(ctx, documentOperationOpen, "Open show", err)
			}
			return
		}
		defer func() { _ = file.Close() }()
		loadedPath := explorerPath(file)
		tmp, err := os.CreateTemp("", "cusus-open-*.cusus")
		if err == nil {
			_, err = io.Copy(tmp, file)
		}
		if tmp != nil {
			err = closeWithError(tmp, err)
			defer func() { _ = os.Remove(tmp.Name()) }()
		}
		if err != nil {
			c.reportFailure(ctx, documentOperationOpen, "Open show", err)
			return
		}
		manifest, files, err := project.Load(tmp.Name())
		if err != nil {
			c.reportFailure(ctx, documentOperationOpen, "Open show", err)
			return
		}
		c.postUI(ctx, func() { c.replaceLoaded(loadedPath, manifest.Show, files) })
	})
}

func (c *documentController) replaceLoaded(path string, current show.Show, files []project.File) {
	c.playback.StopAll()
	c.library.Replace(files)
	c.session.beginReplace()
	c.manager.ReplaceShow(current)
	c.session.finishReplace(path, current)
	c.markJournalSaved(current, path)
	c.panel.SetStatus("Loaded " + documentName(path) + " · recovery journal on")
	c.events.Diagnostic("Open show", "Show loaded and verified", map[string]any{"documentPath": path, "assets": len(files)})
}

func (c *documentController) SaveAs(done func(bool)) {
	c.tasks.Go("save-show-as", func(ctx context.Context) {
		file, err := c.explorer.CreateFile("show.cusus")
		if err != nil {
			if !errors.Is(err, explorer.ErrUserDecline) {
				c.reportFailure(ctx, documentOperationSave, "Save show", err)
			}
			c.complete(ctx, done, false)
			return
		}
		path := explorerPath(file)
		if err := file.Close(); err != nil {
			c.events.Add(operatorlog.Recoverable, "Save show", err.Error(), show.CueID{}, "")
			c.complete(ctx, done, false)
			return
		}
		if strings.TrimSpace(path) == "" {
			c.events.Add(operatorlog.Recoverable, "Save show", "file picker did not return a filesystem path", show.CueID{}, "")
			c.complete(ctx, done, false)
			return
		}
		c.saveAt(ctx, path, true, done)
	})
}

func (c *documentController) Save(done func(bool)) {
	path := c.session.pathSnapshot()
	if path == "" {
		c.SaveAs(done)
		return
	}
	c.tasks.Go("save-show", func(ctx context.Context) { c.saveAt(ctx, path, false, done) })
}

func (c *documentController) saveAt(ctx context.Context, path string, updatePath bool, done func(bool)) {
	if updatePath {
		c.postUI(ctx, func() { c.panel.SetStatus("Saving and optimizing bundled media…") })
	} else {
		c.postUI(ctx, func() { c.panel.SetStatus("Saving " + documentName(path) + "…") })
	}
	snapshot := c.manager.ShowSnapshot()
	var manifest project.Manifest
	var err error
	c.session.serializeSave(func() {
		manifest, err = project.SaveAtPathWithProgress(path, snapshot, c.settings.Snapshot().FFmpegPath, func(progress project.SaveProgress) {
			status := formatSaveProgress(path, progress)
			c.postUI(ctx, func() { c.panel.SetStatus(status) })
		})
	})
	if err != nil {
		source := "Save show"
		if updatePath {
			source = "FFmpeg / save show"
		}
		c.reportFailure(ctx, documentOperationSave, source, err)
		c.complete(ctx, done, false)
		return
	}
	if updatePath {
		c.session.markSaved(path, snapshot)
	} else {
		c.session.markSaved("", snapshot)
	}
	c.markJournalSaved(snapshot, path)
	c.postUI(ctx, func() {
		c.panel.SetStatus("Saved " + documentName(path) + " · recovery journal on · " + formatFileCount(len(manifest.Assets)))
		c.events.Diagnostic("Save show", "Show archive published", map[string]any{"documentPath": path, "assets": len(manifest.Assets)})
	})
	c.complete(ctx, done, true)
}

func (c *documentController) New() {
	c.playback.StopAll()
	c.library.Replace(nil)
	c.session.beginReplace()
	c.manager.ReplaceShow(show.Show{})
	c.session.finishReplace("", show.Show{})
	c.markJournalSaved(show.Show{}, "")
	c.panel.SetStatus("New untitled show · recovery journal on")
}

func (c *documentController) markJournalSaved(current show.Show, path string) {
	if c.journal != nil {
		if err := c.journal.MarkSaved(current, path); err != nil {
			c.events.Add(operatorlog.Recoverable, "Edit recovery", err.Error(), show.CueID{}, "")
		}
	}
}

func (c *documentController) reportFailure(ctx context.Context, operation string, source string, err error) {
	c.events.Add(operatorlog.Recoverable, source, err.Error(), show.CueID{}, "")
	c.postUI(ctx, func() { c.panel.SetStatus(operation + " failed: " + err.Error()) })
}

func (c *documentController) complete(ctx context.Context, done func(bool), success bool) {
	if done != nil {
		c.postUI(ctx, func() { done(success) })
	}
}

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
