package redundancy

import (
	"strings"
	"testing"
	"time"
)

var completePolicyFingerprint = Fingerprint{Show: "show", Media: "media", Routing: "routing", Ready: true}

func TestAuthorityPolicyReconcileDecisions(t *testing.T) {
	now := time.Unix(100, 0)
	tests := []struct {
		name           string
		policy         authorityPolicy
		want           authorityEffect
		wantSplitBrain bool
	}{
		{name: "off idle", policy: policyState(RoleOff, false), want: authorityNoEffect},
		{name: "off owner releases", policy: policyState(RoleOff, true), want: authorityRelease},
		{name: "incomplete idle", policy: policyState(RolePrimary, false, withIncompleteFingerprint()), want: authorityNoEffect},
		{name: "incomplete owner releases", policy: policyState(RolePrimary, true, withIncompleteFingerprint()), want: authorityRelease},
		{name: "primary owner refreshes", policy: policyState(RolePrimary, true), want: authorityRefresh},
		{name: "standby owner refreshes without active peer", policy: policyState(RoleStandby, true), want: authorityRefresh},
		{
			name:   "standby owner fences for fresh active primary",
			policy: policyState(RoleStandby, true, withPeer(now, true)),
			want:   authorityRelease, wantSplitBrain: true,
		},
		{
			name:   "standby owner keeps lock after peer lease expires",
			policy: policyState(RoleStandby, true, withPeer(now.Add(-2*time.Second), true)),
			want:   authorityRefresh,
		},
		{name: "primary acquires by default", policy: policyState(RolePrimary, false), want: authorityAcquire},
		{name: "released primary remains idle", policy: policyState(RolePrimary, false, withReleased()), want: authorityNoEffect},
		{
			name:   "primary waits for fresh active peer",
			policy: policyState(RolePrimary, false, withPeer(now, true)),
			want:   authorityNoEffect,
		},
		{
			name:   "primary reacquires after peer lease expires",
			policy: policyState(RolePrimary, false, withPeer(now.Add(-2*time.Second), true)),
			want:   authorityAcquire,
		},
		{
			name:   "inactive peer does not block primary",
			policy: policyState(RolePrimary, false, withPeer(now, false)),
			want:   authorityAcquire,
		},
		{name: "standby never automatically acquires", policy: policyState(RoleStandby, false), want: authorityNoEffect},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.policy.reconcile(now)
			if got != test.want {
				t.Fatalf("reconcile() = %v, want %v", got, test.want)
			}
			hasSplitBrain := strings.Contains(test.policy.lastError, "split-brain")
			if hasSplitBrain != test.wantSplitBrain {
				t.Fatalf("split-brain error = %v, last error %q", hasSplitBrain, test.policy.lastError)
			}
		})
	}
}

func TestAuthorityPolicyTakeoverDecisions(t *testing.T) {
	now := time.Unix(100, 0)
	tests := []struct {
		name      string
		policy    authorityPolicy
		want      authorityEffect
		wantError string
	}{
		{name: "disabled", policy: policyState(RoleOff, false), wantError: "disabled"},
		{name: "already owner", policy: policyState(RoleStandby, true), want: authorityNoEffect},
		{name: "local fingerprint incomplete", policy: policyState(RoleStandby, false, withIncompleteFingerprint()), wantError: "not ready"},
		{name: "peer never seen", policy: policyState(RoleStandby, false), wantError: "previously validated"},
		{
			name:      "interlock mismatch",
			policy:    policyState(RoleStandby, false, withPeer(now, false), func(p *authorityPolicy) { p.peer.InterlockID = "other" }),
			wantError: "different interlock",
		},
		{
			name:      "fingerprint mismatch",
			policy:    policyState(RoleStandby, false, withPeer(now, false), func(p *authorityPolicy) { p.peer.Fingerprint.Show = "other" }),
			wantError: "fingerprints differ",
		},
		{name: "fresh active peer", policy: policyState(RoleStandby, false, withPeer(now, true)), wantError: "peer still owns"},
		{name: "fresh released peer", policy: policyState(RoleStandby, false, withReleased(), withPeer(now, false)), want: authorityAcquire},
		{name: "expired active peer", policy: policyState(RoleStandby, false, withReleased(), withPeer(now.Add(-2*time.Second), true)), want: authorityAcquire},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.policy.requestTakeover(now)
			if got != test.want {
				t.Fatalf("requestTakeover() effect = %v, want %v", got, test.want)
			}
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("requestTakeover() error = %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("requestTakeover() error = %v, want containing %q", err, test.wantError)
			}
			if got == authorityAcquire && test.policy.released {
				t.Fatal("successful takeover did not clear the explicit-release latch")
			}
		})
	}
}

func TestAuthorityPolicyGateDecisions(t *testing.T) {
	tests := []struct {
		name      string
		policy    authorityPolicy
		wantError string
	}{
		{name: "off bypasses gate", policy: policyState(RoleOff, false, func(p *authorityPolicy) { p.lastError = "ignored" })},
		{name: "failure wins", policy: policyState(RolePrimary, true, func(p *authorityPolicy) { p.lastError = "transport failed" }), wantError: "transport failed"},
		{name: "not owner", policy: policyState(RolePrimary, false), wantError: "does not own"},
		{name: "local incomplete", policy: policyState(RolePrimary, true, withIncompleteFingerprint()), wantError: "not ready"},
		{name: "peer unseen", policy: policyState(RolePrimary, true), wantError: "has not completed"},
		{
			name:      "interlock mismatch",
			policy:    policyState(RolePrimary, true, withPeer(time.Unix(100, 0), false), func(p *authorityPolicy) { p.peer.InterlockID = "other" }),
			wantError: "different interlocks",
		},
		{
			name:      "fingerprint mismatch",
			policy:    policyState(RolePrimary, true, withPeer(time.Unix(100, 0), false), func(p *authorityPolicy) { p.peer.Fingerprint.Show = "other" }),
			wantError: "fingerprints do not match",
		},
		{name: "validated owner", policy: policyState(RolePrimary, true, withPeer(time.Unix(100, 0), false))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.policy.gate()
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("gate() = %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("gate() = %v, want containing %q", err, test.wantError)
			}
		})
	}
}

func TestAuthorityPolicyPeerAcceptanceAndReplayFencing(t *testing.T) {
	now := time.Unix(100, 0)
	policy := policyState(RoleStandby, false)
	base := heartbeat{NodeID: "primary", Role: RolePrimary, BootID: "boot-a", Sequence: 2, SentUnixNano: 20}

	invalid := []struct {
		name    string
		message heartbeat
	}{
		{name: "empty node", message: heartbeat{Role: RolePrimary}},
		{name: "self", message: heartbeat{NodeID: "standby", Role: RolePrimary}},
		{name: "wrong role", message: heartbeat{NodeID: "peer", Role: RoleStandby}},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			candidate := policy
			if candidate.acceptPeer(test.message, now) {
				t.Fatal("invalid heartbeat was accepted")
			}
		})
	}
	if !policy.acceptPeer(base, now) {
		t.Fatal("first valid heartbeat was rejected")
	}
	policy.lastError = "cleared by accepted heartbeat"

	stale := []struct {
		name    string
		message heartbeat
	}{
		{name: "same sequence", message: base},
		{name: "lower sequence", message: heartbeat{NodeID: "primary", Role: RolePrimary, BootID: "boot-a", Sequence: 1, SentUnixNano: 21}},
		{name: "older replacement boot", message: heartbeat{NodeID: "primary", Role: RolePrimary, BootID: "boot-b", Sequence: 1, SentUnixNano: 20}},
	}
	for _, test := range stale {
		t.Run(test.name, func(t *testing.T) {
			if policy.acceptPeer(test.message, now.Add(time.Second)) {
				t.Fatal("stale heartbeat was accepted")
			}
			if policy.peer.BootID != "boot-a" || policy.peer.Sequence != 2 || !policy.lastPeer.Equal(now) {
				t.Fatalf("stale heartbeat mutated peer state: %+v", policy)
			}
		})
	}

	replacement := heartbeat{NodeID: "primary", Role: RolePrimary, BootID: "boot-b", Sequence: 1, SentUnixNano: 21}
	if !policy.acceptPeer(replacement, now.Add(time.Second)) {
		t.Fatal("newer replacement boot was rejected")
	}
	if policy.lastError != "" || policy.peer != replacement || !policy.lastPeer.Equal(now.Add(time.Second)) {
		t.Fatalf("accepted heartbeat state = %+v", policy)
	}
}

func TestAuthorityPolicyStatusStates(t *testing.T) {
	now := time.Unix(100, 0)
	tests := []struct {
		name     string
		policy   authorityPolicy
		want     State
		canIssue bool
	}{
		{name: "off", policy: policyState(RoleOff, false), want: StateOff, canIssue: true},
		{name: "failed", policy: policyState(RolePrimary, true, func(p *authorityPolicy) { p.lastError = "failed" }), want: StateFailed},
		{name: "fingerprint mismatch", policy: policyState(RolePrimary, true, withPeer(now, false), func(p *authorityPolicy) { p.peer.Fingerprint.Show = "other" }), want: StateMismatch},
		{name: "interlock mismatch", policy: policyState(RolePrimary, true, withPeer(now, false), func(p *authorityPolicy) { p.peer.InterlockID = "other" }), want: StateMismatch},
		{name: "active", policy: policyState(RolePrimary, true, withPeer(now, false)), want: StateActive, canIssue: true},
		{name: "ready", policy: policyState(RoleStandby, false, withPeer(now, true)), want: StateReady},
		{name: "waiting for peer", policy: policyState(RoleStandby, false), want: StateWaiting},
		{name: "waiting for expired peer", policy: policyState(RoleStandby, false, withPeer(now.Add(-2*time.Second), true)), want: StateWaiting},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status := test.policy.status(Config{InterlockPath: "lock"}, now, now.Add(-time.Second))
			if status.State != test.want || status.CanIssueCommands != test.canIssue {
				t.Fatalf("status = %+v, want state %s canIssue %v", status, test.want, test.canIssue)
			}
		})
	}
}

func TestAuthorityPolicyReleaseAndAuthorityOutcomes(t *testing.T) {
	off := policyState(RoleOff, true)
	if effect, err := off.requestRelease(); effect != authorityNoEffect || err == nil {
		t.Fatalf("disabled release = (%v, %v)", effect, err)
	}

	owner := policyState(RolePrimary, true)
	if effect, err := owner.requestRelease(); effect != authorityRelease || err != nil || !owner.released {
		t.Fatalf("owner release = (%v, %v), state %+v", effect, err, owner)
	}
	owner.authorityReleased()
	if owner.authority {
		t.Fatal("release outcome retained authority")
	}
	owner.authorityAcquired()
	if !owner.authority || owner.lastError != "" {
		t.Fatalf("acquire outcome = %+v", owner)
	}

	idle := policyState(RoleStandby, false)
	if effect, err := idle.requestRelease(); effect != authorityNoEffect || err != nil || !idle.released {
		t.Fatalf("idle release = (%v, %v), state %+v", effect, err, idle)
	}
}

type policyOption func(*authorityPolicy)

func policyState(role Role, authority bool, options ...policyOption) authorityPolicy {
	policy := authorityPolicy{
		role: role, nodeID: "standby", peerTimeout: time.Second,
		fingerprint: completePolicyFingerprint, interlockID: "lock", authority: authority,
	}
	for _, option := range options {
		option(&policy)
	}
	return policy
}

func withIncompleteFingerprint() policyOption {
	return func(policy *authorityPolicy) { policy.fingerprint = Fingerprint{} }
}

func withReleased() policyOption {
	return func(policy *authorityPolicy) { policy.released = true }
}

func withPeer(lastSeen time.Time, active bool) policyOption {
	return func(policy *authorityPolicy) {
		policy.peerSeen = true
		policy.lastPeer = lastSeen
		policy.peer = heartbeat{
			NodeID: "primary", Role: RolePrimary, Active: active,
			Fingerprint: completePolicyFingerprint, InterlockID: "lock",
		}
	}
}
