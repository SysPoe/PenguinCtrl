//go:build windows

package media

import (
	"testing"

	"github.com/syspoe/cusus/config"
)

func TestMediaWindowPlacement(t *testing.T) {
	displays := []VideoDisplay{
		{ID: "primary", Primary: true, X: 0, Y: 0, Width: 1920, Height: 1080},
		{ID: "stage", X: -1280, Y: 0, Width: 1280, Height: 1024},
	}
	tests := []struct {
		name                       string
		route                      config.VideoOutput
		displays                   []VideoDisplay
		wantX, wantY, wantW, wantH int
		wantFound                  bool
		wantNoGeometry             bool
	}{
		{
			name:      "preserves windowed geometry on selected display",
			route:     config.VideoOutput{DisplayID: "stage", X: 100, Y: 80, Width: 960, Height: 540},
			displays:  displays,
			wantX:     -1180,
			wantY:     80,
			wantW:     960,
			wantH:     540,
			wantFound: true,
		},
		{
			name:      "uses exact selected display for fullscreen",
			route:     config.VideoOutput{DisplayID: "stage", Fullscreen: true, X: 100, Y: 80, Width: 960, Height: 540},
			displays:  displays,
			wantX:     -1280,
			wantY:     0,
			wantW:     1280,
			wantH:     1024,
			wantFound: true,
		},
		{
			name:      "clamps missing-display geometry to primary fallback",
			route:     config.VideoOutput{DisplayID: "disconnected", X: 2500, Y: 1200, Width: 960, Height: 540},
			displays:  displays,
			wantX:     960,
			wantY:     540,
			wantW:     960,
			wantH:     540,
			wantFound: false,
		},
		{
			name:      "fits oversized window to selected display",
			route:     config.VideoOutput{DisplayID: "stage", X: -400, Y: -300, Width: 4000, Height: 3000},
			displays:  displays,
			wantX:     -1280,
			wantY:     0,
			wantW:     1280,
			wantH:     1024,
			wantFound: true,
		},
		{
			name:           "shows without changing geometry while displays are unavailable",
			route:          config.VideoOutput{Width: 960, Height: 540},
			wantNoGeometry: true,
			wantFound:      false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, found := mediaWindowPlacement(test.route, test.displays)
			if found != test.wantFound {
				t.Fatalf("found = %t, want %t", found, test.wantFound)
			}
			if got.flags&swpShowWindow == 0 {
				t.Fatal("placement does not show the window")
			}
			if test.wantNoGeometry {
				if got.flags&(swpNoMove|swpNoSize) != swpNoMove|swpNoSize {
					t.Fatalf("flags = %#x, want no-move and no-size", got.flags)
				}
				return
			}
			if got.x != test.wantX || got.y != test.wantY || got.width != test.wantW || got.height != test.wantH {
				t.Fatalf("geometry = (%d, %d) %dx%d, want (%d, %d) %dx%d", got.x, got.y, got.width, got.height, test.wantX, test.wantY, test.wantW, test.wantH)
			}
		})
	}
}

func TestMediaWindowPlacementPreservesTopmostPolicy(t *testing.T) {
	displays := []VideoDisplay{{Primary: true, Width: 1920, Height: 1080}}
	normal, _ := mediaWindowPlacement(config.VideoOutput{Width: 960, Height: 540}, displays)
	topmost, _ := mediaWindowPlacement(config.VideoOutput{Width: 960, Height: 540, AlwaysOnTop: true}, displays)
	if normal.insertAfter != ^uintptr(1) {
		t.Fatalf("normal insertAfter = %#x, want HWND_NOTOPMOST", normal.insertAfter)
	}
	if topmost.insertAfter != ^uintptr(0) {
		t.Fatalf("topmost insertAfter = %#x, want HWND_TOPMOST", topmost.insertAfter)
	}
}
