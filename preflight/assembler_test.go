package preflight

import (
	"testing"

	"github.com/syspoe/cusus/operatorlog"
)

func TestDiskCautionIsAcknowledgeable(t *testing.T) {
	checks := diskCaution("cache is unavailable")
	if len(checks) != 1 || checks[0].Severity != operatorlog.Warning || checks[0].Fingerprint == "" {
		t.Fatalf("disk caution = %#v", checks)
	}
}
