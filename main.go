package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"gioui.org/x/explorer"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/media"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/playback"
	"github.com/syspoe/cusus/project"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/ui"
)

var topBar ui.TopBar
var playbackSidebar ui.PlaybackSidebar
var tbCtx ui.TBContext
var manager *show.ShowManager = show.NewShowManager()
var settingsStore *config.Store
var playbackEngine *playback.Engine
var mediaManager *media.Manager
var settingsPage *ui.SettingsPage
var projectLibrary = project.NewLibrary()
var showSettings bool

func main() {
	var err error
	settingsStore, err = config.Open("")
	if err != nil {
		log.Fatal(err)
	}
	playbackEngine = playback.NewEngine(manager, settingsStore)
	playbackEngine.SetDurationProbe(func(source string) (int64, error) {
		return media.ProbeDurationMs(settingsStore.Snapshot().FFmpegPath, source)
	})
	playbackEngine.Start()
	mediaManager = media.NewManager(playbackEngine, settingsStore)
	mediaManager.SyncOutputs(playbackEngine.OutputIDs())
	settingsPage = ui.NewSettingsPage(settingsStore)
	settingsPage.SetAudioDeviceProvider(func() ([]ui.AudioDevice, error) {
		devices, err := mediaManager.AudioDevices()
		result := make([]ui.AudioDevice, len(devices))
		for i, device := range devices {
			result[i] = ui.AudioDevice{ID: device.ID, Name: device.Name, IsDefault: device.IsDefault}
		}
		return result, err
	})
	settingsPage.SetOnSaved(func() {
		playbackEngine.RefreshDurations()
		mediaManager.SyncOutputs(playbackEngine.OutputIDs())
	})
	settingsPage.SetOnReopenOutputs(func() {
		mediaManager.EnsureOutputs(playbackEngine.OutputIDs())
	})
	tbCtx = ui.TBContext{
		TopBar:        &topBar,
		TogglePreview: playbackEngine.TogglePreview,
		StopPreview:   playbackEngine.StopPreview,
	}
	go func() {
		window := new(app.Window)
		window.Option(
			app.Title("CuSus"),
			app.Size(unit.Dp(1300), unit.Dp(720)),
			app.MinSize(unit.Dp(1300), unit.Dp(720)),
		)
		if err := run(window); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

func newTheme() *material.Theme {
	th := material.NewTheme()
	th.Face = font.Typeface("Nirmala UI")

	th.Palette.Bg = palette.Background
	th.Palette.ContrastBg = palette.SurfaceRaised
	th.Palette.Fg = palette.TextSoft
	th.Palette.ContrastFg = palette.Text

	return th
}

func handleCueListShortcuts(gtx layout.Context) {
	if showSettings || tbCtx.CueEditorOpen() {
		return
	}
	if topBar.AddCueMenuOpen() || topBar.ActionMenuOpen() || topBar.FileMenuOpen() {
		for {
			event, ok := gtx.Event(key.Filter{Name: key.NameEscape})
			if !ok {
				return
			}
			if event, ok := event.(key.Event); ok && event.State == key.Press {
				topBar.CloseMenus()
				return
			}
		}
	}
	if tbCtx.DeleteConfirmationOpen() {
		tbCtx.HandleDeleteConfirmationKeys(gtx, manager)
		return
	}
	if tbCtx.MoveCueActive() {
		for {
			event, ok := gtx.Event(key.Filter{Name: key.NameEscape})
			if !ok {
				return
			}
			if event, ok := event.(key.Event); ok && event.State == key.Press {
				tbCtx.CancelMoveCue()
				return
			}
		}
	}

	for {
		event, ok := gtx.Event(
			key.Filter{Name: "N", Required: key.ModShortcut},
			key.Filter{Name: "S", Required: key.ModShortcut},
			key.Filter{Name: "S", Required: key.ModShortcut | key.ModShift},
		)
		if !ok {
			break
		}
		keyEvent, ok := event.(key.Event)
		if !ok {
			continue
		}
		if dispatchDocumentShortcut(&topBar, keyEvent) {
			return
		}
	}

	// Let a focused top-bar button retain its normal Enter/Space behavior.
	if topBar.HasKeyboardFocus(gtx) || playbackSidebar.HasKeyboardFocus(gtx) {
		return
	}

	for {
		event, ok := gtx.Event(
			key.Filter{Name: key.NameUpArrow},
			key.Filter{Name: key.NameDownArrow},
			key.Filter{Name: key.NamePageUp},
			key.Filter{Name: key.NamePageDown},
			key.Filter{Name: key.NameHome},
			key.Filter{Name: key.NameEnd},
			key.Filter{Name: key.NameReturn},
			key.Filter{Name: key.NameEnter},
			key.Filter{Name: key.NameF2},
			key.Filter{Name: key.NameSpace},
			key.Filter{Name: key.NameDeleteForward},
			key.Filter{Name: "C", Required: key.ModShortcut},
			key.Filter{Name: "V", Required: key.ModShortcut},
			key.Filter{Name: "D", Required: key.ModShortcut},
			key.Filter{Name: "M", Required: key.ModShortcut},
			key.Filter{Name: "E", Required: key.ModShortcut},
		)
		if !ok {
			return
		}
		keyEvent, ok := event.(key.Event)
		if !ok || keyEvent.State != key.Press {
			continue
		}

		switch keyEvent.Name {
		case key.NameUpArrow:
			manager.MoveSelection(-1)
		case key.NameDownArrow:
			manager.MoveSelection(1)
		case key.NamePageUp:
			manager.MoveSelection(-10)
		case key.NamePageDown:
			manager.MoveSelection(10)
		case key.NameHome:
			manager.SelectCue(0)
		case key.NameEnd:
			manager.SelectCue(manager.CueCount() - 1)
		case key.NameReturn, key.NameEnter, key.NameF2:
			tbCtx.EditSelectedCue(manager)
		case key.NameSpace:
			if err := playbackEngine.PlaySelected(); err != nil {
				log.Printf("play cue: %v", err)
			}
		case key.NameDeleteForward:
			tbCtx.RequestDeleteCue(manager)
		case "C":
			tbCtx.CopySelectedCue(manager)
		case "V":
			tbCtx.PasteCueBeforeSelected(manager)
		case "D":
			tbCtx.DuplicateSelectedCue(manager)
		case "M":
			tbCtx.StartMoveCue(manager)
		case "E":
			tbCtx.EditSelectedCue(manager)
		}
	}
}

func dispatchDocumentShortcut(bar *ui.TopBar, event key.Event) bool {
	if bar == nil || event.State != key.Press || !event.Modifiers.Contain(key.ModShortcut) {
		return false
	}
	switch event.Name {
	case "N":
		bar.RequestNew()
	case "S":
		if event.Modifiers.Contain(key.ModShift) {
			bar.RequestSaveAs()
		} else {
			bar.RequestSave()
		}
	default:
		return false
	}
	return true
}

func layoutFocusWarning(th *material.Theme, gtx layout.Context) layout.Dimensions {
	size := gtx.Constraints.Max
	height := gtx.Dp(unit.Dp(88))
	if height > size.Y {
		height = size.Y
	}
	top := size.Y - height

	paint.FillShape(
		gtx.Ops,
		palette.Danger,
		clip.Rect{Min: image.Pt(0, top), Max: size}.Op(),
	)

	offset := op.Offset(image.Pt(0, top)).Push(gtx.Ops)
	defer offset.Pop()
	gtx.Constraints.Min = image.Pt(size.X, height)
	gtx.Constraints.Max = gtx.Constraints.Min
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		label := material.H3(th, "** WARNING ** NO FOCUS **")
		label.Color = palette.White
		return label.Layout(gtx)
	})
}

func run(window *app.Window) error {
	var ops op.Ops
	windowFocused := true
	th := newTheme()
	expl := explorer.NewExplorer(window)
	uiActions := make(chan func(), 16)
	autosaveRequests := make(chan struct{}, 1)
	var documentMu sync.RWMutex
	var saveMu sync.Mutex
	var currentShowPath string
	var lastSavedDigest [sha256.Size]byte
	var suppressAutosave bool
	tbCtx.LoadWaveform = func(source string, completed func([]float32, int, int64, error)) {
		go func() {
			wave, err := media.ExtractWaveform(settingsStore.Snapshot().FFmpegPath, source)
			uiActions <- func() { completed(wave.Samples, wave.SampleRate, wave.DurationMs, err) }
			window.Invalidate()
		}()
	}
	playbackEngine.SetOnChange(func() {
		window.Invalidate()
		mediaManager.SyncOutputs(playbackEngine.OutputIDs())
	})
	manager.SetOnChange(func() {
		window.Invalidate()
		playbackEngine.RefreshDurations()
		mediaManager.SyncOutputs(playbackEngine.OutputIDs())
		documentMu.RLock()
		shouldAutosave := currentShowPath != "" && !suppressAutosave
		documentMu.RUnlock()
		if shouldAutosave {
			select {
			case autosaveRequests <- struct{}{}:
			default:
			}
		}
	})

	tbCtx.ProjectFiles = func(kind string) []ui.ProjectFile {
		files := projectLibrary.Files(kind)
		result := make([]ui.ProjectFile, len(files))
		for i, file := range files {
			result[i] = ui.ProjectFile{Name: file.Name, Path: file.Source}
		}
		return result
	}
	tbCtx.PickFile = func(kind string, extensions []string, selected func(path string)) {
		go func() {
			file, err := expl.ChooseFile(extensions...)
			if err != nil {
				if !errors.Is(err, explorer.ErrUserDecline) {
					log.Printf("pick file: %v", err)
				}
				return
			}
			defer file.Close()

			path := ""
			switch f := file.(type) {
			case *explorer.File:
				path = f.URI()
			case *os.File:
				path = f.Name()
			}
			if path == "" || selected == nil {
				return
			}
			entry, duplicate, err := projectLibrary.Add(path, kind)
			if err != nil {
				uiActions <- func() { topBar.SetStatus("Could not add file: " + err.Error()) }
				window.Invalidate()
				return
			}

			uiActions <- func() {
				selected(entry.Source)
				if duplicate {
					topBar.SetStatus("Duplicate detected · using existing " + entry.Name)
				} else {
					topBar.SetStatus("Added " + entry.Name)
				}
			}
			window.Invalidate()
		}()
	}

	loadShow := func() {
		go func() {
			file, err := expl.ChooseFile(".cusus")
			if err != nil {
				if !errors.Is(err, explorer.ErrUserDecline) {
					uiActions <- func() { topBar.SetStatus("Open failed: " + err.Error()) }
					window.Invalidate()
				}
				return
			}
			defer file.Close()
			loadedPath := explorerPath(file)
			tmp, err := os.CreateTemp("", "cusus-open-*.cusus")
			if err == nil {
				_, err = io.Copy(tmp, file)
			}
			if tmp != nil {
				tmp.Close()
				defer os.Remove(tmp.Name())
			}
			if err != nil {
				uiActions <- func() { topBar.SetStatus("Open failed: " + err.Error()) }
				window.Invalidate()
				return
			}
			manifest, files, err := project.Load(tmp.Name())
			if err != nil {
				uiActions <- func() { topBar.SetStatus("Open failed: " + err.Error()) }
				window.Invalidate()
				return
			}
			uiActions <- func() {
				playbackEngine.StopAll()
				projectLibrary.Replace(files)
				documentMu.Lock()
				suppressAutosave = true
				documentMu.Unlock()
				manager.ReplaceShow(manifest.Show)
				documentMu.Lock()
				currentShowPath = loadedPath
				lastSavedDigest = showDigest(manifest.Show)
				suppressAutosave = false
				documentMu.Unlock()
				topBar.SetStatus("Loaded " + documentName(loadedPath) + " · autosave on")
			}
			window.Invalidate()
		}()
	}

	var saveAsShow func()
	var saveShow func()

	saveAsShow = func() {
		go func() {
			file, err := expl.CreateFile("show.cusus")
			if err != nil {
				if !errors.Is(err, explorer.ErrUserDecline) {
					uiActions <- func() { topBar.SetStatus("Save failed: " + err.Error()) }
					window.Invalidate()
				}
				return
			}
			path := explorerPath(file)
			uiActions <- func() { topBar.SetStatus("Saving and optimizing bundled media…") }
			window.Invalidate()
			snapshot := manager.ShowSnapshot()
			saveMu.Lock()
			manifest, err := project.Save(file, snapshot, settingsStore.Snapshot().FFmpegPath)
			closeErr := file.Close()
			saveMu.Unlock()
			if err == nil {
				err = closeErr
			}
			if err != nil {
				uiActions <- func() { topBar.SetStatus("Save failed: " + err.Error()) }
				window.Invalidate()
				return
			}
			documentMu.Lock()
			currentShowPath = path
			lastSavedDigest = showDigest(snapshot)
			documentMu.Unlock()
			uiActions <- func() {
				topBar.SetStatus("Saved " + documentName(path) + " · autosave on · " + formatFileCount(len(manifest.Assets)))
			}
			window.Invalidate()
		}()
	}

	saveShow = func() {
		documentMu.RLock()
		path := currentShowPath
		documentMu.RUnlock()
		if path == "" {
			saveAsShow()
			return
		}
		go func() {
			snapshot := manager.ShowSnapshot()
			uiActions <- func() { topBar.SetStatus("Saving " + documentName(path) + "…") }
			window.Invalidate()
			saveMu.Lock()
			manifest, err := saveShowAtPath(path, snapshot, settingsStore.Snapshot().FFmpegPath)
			saveMu.Unlock()
			if err != nil {
				uiActions <- func() { topBar.SetStatus("Save failed: " + err.Error()) }
				window.Invalidate()
				return
			}
			documentMu.Lock()
			lastSavedDigest = showDigest(snapshot)
			documentMu.Unlock()
			uiActions <- func() {
				topBar.SetStatus("Saved " + documentName(path) + " · autosave on · " + formatFileCount(len(manifest.Assets)))
			}
			window.Invalidate()
		}()
	}

	go func() {
		for range autosaveRequests {
			timer := time.NewTimer(1200 * time.Millisecond)
		debounce:
			for {
				select {
				case <-autosaveRequests:
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					timer.Reset(1200 * time.Millisecond)
				case <-timer.C:
					break debounce
				}
			}
			snapshot := manager.ShowSnapshot()
			digest := showDigest(snapshot)
			documentMu.RLock()
			path, alreadySaved := currentShowPath, digest == lastSavedDigest
			documentMu.RUnlock()
			if path == "" || alreadySaved {
				continue
			}
			saveMu.Lock()
			_, err := saveShowAtPath(path, snapshot, settingsStore.Snapshot().FFmpegPath)
			saveMu.Unlock()
			if err != nil {
				uiActions <- func() { topBar.SetStatus("Autosave failed: " + err.Error()) }
			} else {
				documentMu.Lock()
				lastSavedDigest = digest
				documentMu.Unlock()
				uiActions <- func() { topBar.SetStatus("Autosaved " + documentName(path)) }
			}
			window.Invalidate()
		}
	}()

	for {
		e := window.Event()
		expl.ListenEvents(e)
		for {
			select {
			case action := <-uiActions:
				action()
			default:
				goto actionsDone
			}
		}
	actionsDone:

		switch e := e.(type) {
		case app.DestroyEvent:
			playbackEngine.Close()
			return e.Err

		case app.ConfigEvent:
			windowFocused = e.Config.Focused
			window.Invalidate()

		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			handleCueListShortcuts(gtx)
			if topBar.TakeNewRequest() {
				playbackEngine.StopAll()
				projectLibrary.Replace(nil)
				documentMu.Lock()
				suppressAutosave = true
				documentMu.Unlock()
				manager.ReplaceShow(show.Show{})
				documentMu.Lock()
				currentShowPath = ""
				lastSavedDigest = showDigest(show.Show{})
				suppressAutosave = false
				documentMu.Unlock()
				topBar.SetStatus("New untitled show · choose Save to start autosave")
			}
			if topBar.TakeLoadRequest() {
				loadShow()
			}
			if topBar.TakeSaveRequest() {
				saveShow()
			}
			if topBar.TakeSaveAsRequest() {
				saveAsShow()
			}
			paint.Fill(gtx.Ops, th.Bg)

			layout.Stack{}.Layout(gtx,
				// Main application layout
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{
						Axis: layout.Vertical,
					}.Layout(gtx,
						// Top Bar
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return topBar.Layout(th, gtx, manager.HasSelectedCue(), showSettings)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							if showSettings {
								return settingsPage.Layout(th, gtx)
							}
							return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return playbackSidebar.Layout(th, gtx, manager, playbackEngine)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									width := gtx.Dp(unit.Dp(1))
									paint.FillShape(gtx.Ops, th.ContrastBg, clip.Rect{Max: image.Pt(width, gtx.Constraints.Max.Y)}.Op())
									return layout.Dimensions{Size: image.Pt(width, gtx.Constraints.Max.Y)}
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return ui.Main(
										th, gtx, manager, playbackEngine,
										func() { tbCtx.EditSelectedCue(manager) },
										tbCtx.MoveCueActive(),
										func(index int) { tbCtx.MoveSelectedCueBefore(manager, index) },
										func() { tbCtx.MoveSelectedCueToEnd(manager) },
									)
								}),
							)
						}),
					)
				}),
				// Top Bar Submenus
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					if showSettings {
						return layout.Dimensions{}
					}
					return tbCtx.Layout(th, gtx, manager)
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return topBar.LayoutFileMenu(th, gtx)
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					if windowFocused {
						return layout.Dimensions{}
					}
					return layoutFocusWarning(th, gtx)
				}),
			)
			if topBar.TakePageRequest() {
				showSettings = !showSettings
				window.Invalidate()
			}
			e.Frame(gtx.Ops)
		}
	}
}

func formatFileCount(count int) string {
	if count == 1 {
		return "1 media file"
	}
	return fmt.Sprintf("%d media files", count)
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

func showDigest(current show.Show) [sha256.Size]byte {
	raw, _ := json.Marshal(current)
	return sha256.Sum256(raw)
}

func saveShowAtPath(path string, current show.Show, ffmpegPath string) (project.Manifest, error) {
	if strings.TrimSpace(path) == "" {
		return project.Manifest{}, errors.New("show has no file path; use Save As")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return project.Manifest{}, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".cusus-save-*")
	if err != nil {
		return project.Manifest{}, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	manifest, err := project.Save(tmp, current, ffmpegPath)
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return project.Manifest{}, err
	}
	if err := os.Rename(tmpPath, path); err == nil {
		return manifest, nil
	}

	// Windows does not consistently replace an existing file with Rename.
	// Keep the old document as a short-lived backup until the new one lands.
	backup := path + ".autosave-backup"
	_ = os.Remove(backup)
	if err := os.Rename(path, backup); err != nil {
		return project.Manifest{}, fmt.Errorf("replace show file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Rename(backup, path)
		return project.Manifest{}, fmt.Errorf("replace show file: %w", err)
	}
	_ = os.Remove(backup)
	return manifest, nil
}
