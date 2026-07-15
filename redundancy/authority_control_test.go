package redundancy

import (
	"strings"
	"testing"
)

func TestAuthorityControlBlocksHandoffWhilePlaybackIsActive(t *testing.T) {
	service := NewService(Config{Role: RoleOff})
	defer service.Close()
	control := NewAuthorityControl(service, func() bool { return true }, nil)

	if err := control.RequestTakeover(); err == nil || !strings.Contains(err.Error(), "STOP all local cues") {
		t.Fatalf("takeover error = %v", err)
	}
	if err := control.ReleaseAuthority(); err == nil || !strings.Contains(err.Error(), "STOP all local cues") {
		t.Fatalf("release error = %v", err)
	}
}

func TestAuthorityControlStopsActivePlaybackWhenConfigureLosesAuthority(t *testing.T) {
	service := NewService(Config{Role: RoleOff})
	defer service.Close()
	service.mu.Lock()
	service.config.Role = RolePrimary
	service.policy.role = RolePrimary
	service.policy.authority = true
	service.mu.Unlock()
	stops := 0
	control := NewAuthorityControl(service, func() bool { return true }, func() { stops++ })

	stopped, err := control.Configure(Config{Role: RoleOff})
	if err != nil {
		t.Fatal(err)
	}
	if !stopped || stops != 1 {
		t.Fatalf("stopped = %t, stops = %d; want true, 1", stopped, stops)
	}
}

func TestAuthorityControlDoesNotStopIdlePlayback(t *testing.T) {
	service := NewService(Config{Role: RoleOff})
	defer service.Close()
	service.mu.Lock()
	service.config.Role = RolePrimary
	service.policy.role = RolePrimary
	service.policy.authority = true
	service.mu.Unlock()
	stops := 0
	control := NewAuthorityControl(service, func() bool { return false }, func() { stops++ })

	stopped, err := control.Configure(Config{Role: RoleOff})
	if err != nil {
		t.Fatal(err)
	}
	if stopped || stops != 0 {
		t.Fatalf("stopped = %t, stops = %d; want false, 0", stopped, stops)
	}
}
