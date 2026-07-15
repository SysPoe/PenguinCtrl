package timecode

import (
	"sync"
	"testing"
	"time"
)

func TestServiceSerializesConcurrentConfigure(t *testing.T) {
	service := NewService(Config{Source: SourceInternal}, "")
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			service.Configure(Config{Source: SourceInternal}, "")
		}()
	}
	wg.Wait()
	service.Close()
}

func TestServiceOwnsLTCAndMTCInputLifecycles(t *testing.T) {
	service := NewService(Config{Source: SourceLTC, Policy: PolicyResync}, "")
	defer service.Close()
	if got := serviceInputStatus(t, service.Status(), SourceLTC).State; got != InputRunning {
		t.Fatalf("LTC state = %q; want running", got)
	}
	if err := service.LTCAdapter().IngestFrame(1, 2, 3, 15, 30); err != nil {
		t.Fatal(err)
	}
	if got := service.Coordinator().Position(); got < time.Hour+2*time.Minute+3500*time.Millisecond || got > time.Hour+2*time.Minute+3510*time.Millisecond {
		t.Fatalf("LTC position = %v", got)
	}

	service.Configure(Config{Source: SourceMTC, Policy: PolicyResync}, "")
	status := service.Status()
	if got := serviceInputStatus(t, status, SourceLTC).State; got != InputStopped {
		t.Fatalf("replaced LTC state = %q; want stopped", got)
	}
	if got := serviceInputStatus(t, status, SourceMTC).State; got != InputRunning {
		t.Fatalf("MTC state = %q; want running", got)
	}
	values := []byte{4, 0, 3, 0, 2, 0, 1, 6}
	for part, value := range values {
		if err := service.MTCAdapter().IngestQuarterFrame(byte(part<<4) | value); err != nil {
			t.Fatal(err)
		}
	}
	want := time.Hour + 2*time.Minute + 3*time.Second + time.Second*4/30
	if got := service.Coordinator().Position(); absDuration(got-want) > 2*time.Millisecond {
		t.Fatalf("MTC position = %v; want %v", got, want)
	}

	service.Configure(Config{Source: SourceInternal}, "")
	status = service.Status()
	if status.Selected != SourceInternal {
		t.Fatalf("selected source = %q; want internal", status.Selected)
	}
	for _, input := range status.Inputs {
		if input.State != InputStopped {
			t.Fatalf("input %q state = %q; want stopped", input.Source, input.State)
		}
	}
}

func TestServiceReportsOSCInputLifecycleFailure(t *testing.T) {
	service := NewService(Config{Source: SourceOSC}, "not-a-valid-listen-address")
	defer service.Close()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && serviceInputStatus(t, service.Status(), SourceOSC).State != InputFailed {
		time.Sleep(time.Millisecond)
	}
	status := serviceInputStatus(t, service.Status(), SourceOSC)
	if status.State != InputFailed || status.LastError == nil || service.LastError() == nil {
		t.Fatalf("OSC failure status = %#v; service error = %v", status, service.LastError())
	}
	service.Close()
	closed := service.Status()
	if !closed.Closed || serviceInputStatus(t, closed, SourceOSC).State != InputStopped {
		t.Fatalf("closed service status = %#v", closed)
	}
}

func TestServiceStartsAndStopsOSCInput(t *testing.T) {
	service := NewService(Config{Source: SourceOSC}, "127.0.0.1:0")
	if got := serviceInputStatus(t, service.Status(), SourceOSC).State; got != InputRunning {
		t.Fatalf("OSC state = %q; want running", got)
	}
	service.Close()
	status := service.Status()
	if !status.Closed || serviceInputStatus(t, status, SourceOSC).State != InputStopped {
		t.Fatalf("closed OSC service = %#v", status)
	}
}

func serviceInputStatus(t *testing.T, status ServiceStatus, source Source) InputStatus {
	t.Helper()
	for _, input := range status.Inputs {
		if input.Source == source {
			return input
		}
	}
	t.Fatalf("service status has no %q input: %#v", source, status)
	return InputStatus{}
}
