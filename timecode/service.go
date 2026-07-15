package timecode

import (
	"context"
	"sync"
)

// Service owns optional external input listeners while exposing one stable
// coordinator to the playback engine and hardware adapters.
// TODO(macro): Service only starts the OSC UDP listener; LTC/MTC remain bare
// Coordinator.Ingest* APIs with no listener lifecycle here. Either host all
// selected Source adapters under Service.Configure (matching the comment) or
// drop the Service layer and let callers own per-source adapters explicitly.
type Service struct {
	configureMu sync.Mutex
	mu          sync.Mutex
	coordinator *Coordinator
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	closed      bool
	lastError   error
}

func NewService(config Config, listenAddress string) *Service {
	service := &Service{coordinator: New(config)}
	service.Configure(config, listenAddress)
	return service
}

func (s *Service) Coordinator() *Coordinator { return s.coordinator }

func (s *Service) Configure(config Config, listenAddress string) {
	s.configureMu.Lock()
	defer s.configureMu.Unlock()

	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.coordinator.Configure(config)
	s.lastError = nil
	if config.Source != SourceOSC {
		s.cancel = nil
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		err := ListenOSC(ctx, listenAddress, s.coordinator)
		s.mu.Lock()
		if ctx.Err() == nil {
			s.lastError = err
		}
		s.mu.Unlock()
	}()
}

func (s *Service) LastError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastError
}

func (s *Service) Close() {
	s.configureMu.Lock()
	defer s.configureMu.Unlock()

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
}
