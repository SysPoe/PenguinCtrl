package media

import (
	"github.com/syspoe/cusus/config"
	"strings"
	"testing"
)

func TestVideoOutputWarning(t *testing.T) {
	settings := config.Settings{OutputSettings: config.OutputSettings{VideoOutputs: []config.VideoOutput{{Stage: "main", DisplayID: "connected"}, {Stage: "foh", DisplayID: "missing"}}}}
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

func TestVideoOutputWarningRequiresConfirmationAndRefreshMatch(t *testing.T) {
	displays := []VideoDisplay{{ID: "stage", RefreshRate: 60}}
	settings := config.Settings{OutputSettings: config.OutputSettings{VideoOutputs: []config.VideoOutput{{Stage: "main", DisplayID: "stage", ExpectedRefresh: 50}}}}
	if warning := videoOutputWarning(settings, displays); !strings.Contains(warning, "confirmation") {
		t.Fatalf("unconfirmed warning = %q", warning)
	}
	settings.VideoOutputs[0].DisplayConfirmed = true
	if warning := videoOutputWarning(settings, displays); !strings.Contains(warning, "50 Hz") || !strings.Contains(warning, "60 Hz") {
		t.Fatalf("refresh warning = %q", warning)
	}
}

func TestDisplaySignatureIncludesOperatorVisibleTopology(t *testing.T) {
	base := []VideoDisplay{{ID: "stage", Name: "Projector", Primary: false}}
	if displaySignature(base) == displaySignature([]VideoDisplay{{ID: "stage", Name: "LED Wall", Primary: false}}) {
		t.Fatal("display name change did not change signature")
	}
	if displaySignature(base) == displaySignature([]VideoDisplay{{ID: "stage", Name: "Projector", Primary: true}}) {
		t.Fatal("primary display change did not change signature")
	}
}
