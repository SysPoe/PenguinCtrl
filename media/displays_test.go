package media

import (
	"github.com/syspoe/cusus/config"
	"strings"
	"testing"
)

func TestVideoOutputWarning(t *testing.T) {
	settings := config.Settings{VideoOutputs: []config.VideoOutput{{Stage: "main", DisplayID: "connected"}, {Stage: "foh", DisplayID: "missing"}}}
	warning := videoOutputWarning(settings, []VideoDisplay{{ID: "connected"}})
	if !strings.Contains(warning, "foh") || strings.Contains(warning, "main") {
		t.Fatalf("unexpected warning %q", warning)
	}
}

func TestResolveDisplayForGeometryFallsBackToPrimary(t *testing.T) {
	displays := []VideoDisplay{{ID: "secondary"}, {ID: "primary", Primary: true, X: 100}}
	display, found := resolveDisplayForGeometry("missing", displays)
	if found || display.ID != "primary" {
		t.Fatalf("got %+v, found=%v", display, found)
	}
}
