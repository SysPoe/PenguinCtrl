package cuevars

import (
	"testing"

	"github.com/syspoe/cusus/config"
)

func TestResolveBuiltinsNestedVariablesAndCueOffsets(t *testing.T) {
	settings := config.Defaults()
	settings.Variables = map[string]string{"next": "{cueNumber+0.5}", "route": "{defaultMediaOutput}"}
	if got := Resolve("{next} on {route}", settings, "12.25"); got != "12.75 on main" {
		t.Fatalf("resolved = %q", got)
	}
}

func TestResolveLeavesUnknownAndInvalidOffsetsVisible(t *testing.T) {
	settings := config.Defaults()
	if got := Resolve("{missing} {cueNumber+0.001}", settings, "1"); got != "{missing} {cueNumber+0.001}" {
		t.Fatalf("resolved = %q", got)
	}
}
