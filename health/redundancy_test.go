package health

import (
	"strings"
	"testing"

	"github.com/syspoe/cusus/redundancy"
)

func TestRedundancySummaryFormatsMachineState(t *testing.T) {
	tests := []struct {
		status redundancy.Status
		want   string
	}{
		{status: redundancy.Status{Role: redundancy.RoleOff}, want: "disabled"},
		{status: redundancy.Status{Role: redundancy.RolePrimary, State: redundancy.StateFailed, LastError: "socket failed"}, want: "socket failed"},
		{status: redundancy.Status{Role: redundancy.RolePrimary, Authority: true, CanIssueCommands: true, PeerFresh: true, PeerNodeID: "standby"}, want: "peer standby is validated"},
		{status: redundancy.Status{Role: redundancy.RoleStandby, PeerActive: true, PeerFresh: true, PeerNodeID: "primary"}, want: "primary owns command authority"},
	}
	for _, test := range tests {
		if got := RedundancySummary(test.status); !strings.Contains(got, test.want) {
			t.Fatalf("summary = %q, want containing %q", got, test.want)
		}
	}
}
