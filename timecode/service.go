package timecode

import (
	"sync"
)

// Service owns the selected OSC, LTC, or MTC input lifecycle while exposing one
// stable coordinator to the playback engine. LTC and MTC decoder integrations
// feed the stable adapters returned by LTCAdapter and MTCAdapter.
type Service struct {
	configureMu sync.Mutex
	mu          sync.RWMutex
	coordinator *Coordinator
	inputs      *inputRegistry
	closed      bool
}

func NewService(config Config, listenAddress string) *Service {
	coordinator := New(config)
	service := &Service{coordinator: coordinator, inputs: newInputRegistry(coordinator)}
	service.Configure(config, listenAddress)
	return service
}

func (s *Service) Coordinator() *Coordinator { return s.coordinator }

func (s *Service) LTCAdapter() *LTCAdapter { return s.inputs.ltc }

func (s *Service) MTCAdapter() *MTCAdapter { return s.inputs.mtc }

func (s *Service) Configure(config Config, listenAddress string) {
	s.configureMu.Lock()
	defer s.configureMu.Unlock()

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	s.inputs.Close()
	s.coordinator.Configure(config)
	s.inputs.Start(normalizeConfig(config).Source, listenAddress)
}

func (s *Service) LastError() error {
	return s.inputs.LastError()
}

func (s *Service) Status() ServiceStatus {
	s.mu.RLock()
	closed := s.closed
	s.mu.RUnlock()
	return ServiceStatus{Selected: s.coordinator.Status().Source, Closed: closed, Inputs: s.inputs.Statuses()}
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
	s.mu.Unlock()
	s.inputs.Close()
}
