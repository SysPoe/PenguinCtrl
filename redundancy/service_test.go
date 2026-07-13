package redundancy

import (
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWarmSparePlannedHandoffAndReturnToPrimary(t *testing.T) {
	primary, standby := newTestPair(t)
	defer primary.Close()
	defer standby.Close()

	waitStatus(t, primary, func(status Status) bool { return status.CanIssueCommands && status.PeerFresh })
	waitStatus(t, standby, func(status Status) bool { return status.State == StateReady && status.PeerActive })
	if err := standby.RequestTakeover(); err == nil || !strings.Contains(err.Error(), "peer still owns") {
		t.Fatalf("standby takeover while primary active = %v", err)
	}

	if err := primary.ReleaseAuthority(); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, standby, func(status Status) bool { return status.PeerFresh && !status.PeerActive })
	if err := standby.RequestTakeover(); err != nil {
		t.Fatalf("planned standby takeover: %v", err)
	}
	waitStatus(t, standby, func(status Status) bool { return status.CanIssueCommands })
	waitStatus(t, primary, func(status Status) bool { return status.PeerActive && !status.Authority })

	if err := standby.ReleaseAuthority(); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, primary, func(status Status) bool { return status.PeerFresh && !status.PeerActive })
	if err := primary.RequestTakeover(); err != nil {
		t.Fatalf("return to primary: %v", err)
	}
	waitStatus(t, primary, func(status Status) bool { return status.CanIssueCommands })
	if standby.Status().Authority {
		t.Fatal("standby retained authority after return to primary")
	}
}

func TestStandbyCanTakeOverAfterHeartbeatExpiryButNotBefore(t *testing.T) {
	primary, standby := newTestPair(t)
	defer standby.Close()
	waitStatus(t, primary, func(status Status) bool { return status.CanIssueCommands })
	waitStatus(t, standby, func(status Status) bool { return status.PeerFresh && status.PeerActive })

	primary.Close()
	if err := standby.RequestTakeover(); err == nil || !strings.Contains(err.Error(), "peer still owns") {
		t.Fatalf("takeover before heartbeat lease expiry = %v", err)
	}
	waitStatus(t, standby, func(status Status) bool { return status.PeerSeen && !status.PeerFresh })
	if err := standby.RequestTakeover(); err != nil {
		t.Fatalf("takeover after heartbeat expiry: %v", err)
	}
	if err := standby.Gate(); err != nil {
		t.Fatalf("standby command gate after fenced takeover: %v", err)
	}
}

func TestHeartbeatPartitionDoesNotOverrideLiveInterlock(t *testing.T) {
	primary, standby := newTestPair(t)
	defer primary.Close()
	defer standby.Close()
	waitStatus(t, primary, func(status Status) bool { return status.CanIssueCommands })
	waitStatus(t, standby, func(status Status) bool { return status.PeerFresh && status.PeerActive })

	primary.mu.Lock()
	if primary.conn == nil {
		primary.mu.Unlock()
		t.Fatal("primary heartbeat socket is unavailable")
	}
	_ = primary.conn.Close()
	primary.mu.Unlock()
	waitStatus(t, standby, func(status Status) bool { return status.PeerSeen && !status.PeerFresh })
	if err := standby.RequestTakeover(); err == nil || !strings.Contains(err.Error(), "interlock remains owned") {
		t.Fatalf("partitioned standby takeover = %v", err)
	}
	if !primary.Status().Authority {
		t.Fatal("network partition unexpectedly released the primary interlock")
	}
}

func TestFingerprintMismatchBlocksCommandIssueAndTakeover(t *testing.T) {
	primaryAddress, standbyAddress := testUDPAddress(t), testUDPAddress(t)
	interlock := filepath.Join(t.TempDir(), "command.lock")
	primary := NewService(testConfig(RolePrimary, "primary", primaryAddress, standbyAddress, interlock))
	standby := NewService(testConfig(RoleStandby, "standby", standbyAddress, primaryAddress, interlock))
	defer primary.Close()
	defer standby.Close()
	primary.UpdateFingerprint(Fingerprint{Show: "show-a", Media: "media", Routing: "routing", Ready: true})
	standby.UpdateFingerprint(Fingerprint{Show: "show-b", Media: "media", Routing: "routing", Ready: true})
	waitStatus(t, primary, func(status Status) bool { return status.PeerSeen && !status.FingerprintsMatch })
	if err := primary.Gate(); err == nil || !strings.Contains(err.Error(), "fingerprints do not match") {
		t.Fatalf("primary mismatch gate = %v", err)
	}
	if err := primary.ReleaseAuthority(); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, standby, func(status Status) bool { return status.PeerSeen && !status.PeerActive })
	if err := standby.RequestTakeover(); err == nil || !strings.Contains(err.Error(), "fingerprints differ") {
		t.Fatalf("mismatched takeover = %v", err)
	}
}

func TestInterlockPreventsConcurrentOwners(t *testing.T) {
	path := filepath.Join(t.TempDir(), "command.lock")
	first, err := acquireSystemInterlock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if _, err := acquireSystemInterlock(path); err != ErrInterlockBusy {
		t.Fatalf("second interlock acquisition = %v", err)
	}
}

func TestHeartbeatAuthenticationRejectsMutation(t *testing.T) {
	message := heartbeat{Version: protocolVersion, NodeID: "primary", Role: RolePrimary, BootID: "boot", Sequence: 1, SentUnixNano: 1, Active: true}
	message.Signature = signHeartbeat(message, "0123456789abcdef")
	if !verifyHeartbeat(message, "0123456789abcdef") {
		t.Fatal("valid heartbeat signature rejected")
	}
	message.Active = false
	if verifyHeartbeat(message, "0123456789abcdef") {
		t.Fatal("mutated heartbeat signature accepted")
	}
}

func TestAuthorityCannotReleaseDuringRemoteDispatch(t *testing.T) {
	primary, standby := newTestPair(t)
	defer primary.Close()
	defer standby.Close()
	waitStatus(t, primary, func(status Status) bool { return status.CanIssueCommands })

	started, finish := make(chan struct{}), make(chan struct{})
	dispatched := make(chan error, 1)
	go func() {
		dispatched <- primary.WithAuthority(func() error {
			close(started)
			<-finish
			return nil
		})
	}()
	<-started
	released := make(chan error, 1)
	go func() { released <- primary.ReleaseAuthority() }()
	select {
	case err := <-released:
		t.Fatalf("authority released during dispatch: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(finish)
	if err := <-dispatched; err != nil {
		t.Fatal(err)
	}
	if err := <-released; err != nil {
		t.Fatal(err)
	}
}

func TestStandbySelfFencesIfActivePrimaryHeartbeatAppears(t *testing.T) {
	path := filepath.Join(t.TempDir(), "standby.lock")
	config := normalizeConfig(testConfig(RoleStandby, "standby", "127.0.0.1:1", "127.0.0.1:2", path))
	fingerprint := Fingerprint{Show: "show", Media: "media", Routing: "routing", Ready: true}
	lock, err := acquireSystemInterlock(path)
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{
		config: config, fingerprint: fingerprint, interlockID: interlockIdentity(path),
		lock: lock, authority: true, peerSeen: true, lastPeer: time.Now(),
		peer: heartbeat{NodeID: "primary", Role: RolePrimary, Active: true, Fingerprint: fingerprint, InterlockID: interlockIdentity(path)},
	}
	service.mu.Lock()
	service.reconcileLocked(time.Now())
	service.mu.Unlock()
	status := service.Status()
	if status.Authority || !strings.Contains(status.LastError, "split-brain") {
		t.Fatalf("split-brain self-fence status = %+v", status)
	}
}

func newTestPair(t *testing.T) (*Service, *Service) {
	t.Helper()
	primaryAddress, standbyAddress := testUDPAddress(t), testUDPAddress(t)
	interlock := filepath.Join(t.TempDir(), "command.lock")
	primary := NewService(testConfig(RolePrimary, "primary", primaryAddress, standbyAddress, interlock))
	standby := NewService(testConfig(RoleStandby, "standby", standbyAddress, primaryAddress, interlock))
	fingerprint := Fingerprint{Show: "show", Media: "media", Routing: "routing", Ready: true}
	primary.UpdateFingerprint(fingerprint)
	standby.UpdateFingerprint(fingerprint)
	return primary, standby
}

func testConfig(role Role, nodeID, listen, peer, interlock string) Config {
	return Config{
		Role: role, NodeID: nodeID, ListenAddress: listen, PeerAddress: peer,
		SharedKey: "0123456789abcdef-test", InterlockPath: interlock,
		HeartbeatInterval: 30 * time.Millisecond, PeerTimeout: 120 * time.Millisecond,
	}
}

func testUDPAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	address := listener.LocalAddr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func waitStatus(t *testing.T, service *Service, ready func(Status) bool) Status {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		status := service.Status()
		if ready(status) {
			return status
		}
		time.Sleep(10 * time.Millisecond)
	}
	status := service.Status()
	t.Fatalf("timed out waiting for redundancy status: %+v", status)
	return status
}
