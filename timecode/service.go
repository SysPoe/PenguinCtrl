package timecode

import (
	"context"
	"sync"
)

// Service owns optional external input listeners while exposing one stable
// coordinator to the playback engine and hardware adapters.
type Service struct {
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
