package health

import (
	"fmt"

	"github.com/syspoe/cusus/redundancy"
)

// RedundancySummary formats machine-readable redundancy state for operators.
func RedundancySummary(status redundancy.Status) string {
	if status.Role == redundancy.RoleOff {
		return "Redundancy is disabled; this node has local command authority"
	}
	if status.LastError != "" && status.State == redundancy.StateFailed {
		return "Redundancy failed: " + status.LastError
	}
	role := redundancyRoleTitle(status.Role)
	if status.Authority && status.CanIssueCommands {
		if status.PeerFresh {
			return fmt.Sprintf("%s owns command authority; peer %s is validated", role, status.PeerNodeID)
		}
		return role + " owns command authority; validated peer heartbeat is stale"
	}
	if status.Authority {
		return role + " owns the interlock but command issue is blocked until both nodes match"
	}
	if status.PeerActive && status.PeerFresh {
		return fmt.Sprintf("Warm spare ready; %s owns command authority", status.PeerNodeID)
	}
	if status.PeerSeen && !status.FingerprintsMatch {
		return "Warm-spare fingerprints do not match"
	}
	if status.PeerSeen && !status.InterlockMatch {
		return "Warm-spare nodes do not use the same interlock"
	}
	return "Waiting for a validated peer and command authority"
}

func redundancyRoleTitle(role redundancy.Role) string {
	switch role {
	case redundancy.RolePrimary:
		return "Primary"
	case redundancy.RoleStandby:
		return "Standby"
	default:
		return "Node"
	}
}
