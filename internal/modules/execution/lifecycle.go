package execution

import (
	"context"
	"errors"
)

func (s *Service) register(id string, cancel context.CancelFunc, ctx context.Context) error {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	if s.closed {
		return errors.New("execution service is closed")
	}
	if s.reconfiguring {
		return errors.New("execution service is applying a composition change")
	}
	if s.active == nil {
		s.active = map[string]context.CancelFunc{}
	}
	if s.active[id] != nil {
		return errors.New("run is already active")
	}
	if s.contexts == nil {
		s.contexts = map[string]context.Context{}
	}
	s.contexts[id] = ctx
	s.active[id] = cancel
	s.activeWG.Add(1)
	return nil
}
func (s *Service) unregister(id string) {
	s.activeMu.Lock()
	delete(s.active, id)
	delete(s.contexts, id)
	s.activeMu.Unlock()
	s.activeWG.Done()
}

// Close drains complete application executions (including metadata writes and
// blocked event consumers) before dependent runners, providers and stores close.
func (s *Service) Close(ctx context.Context) error {
	s.activeMu.Lock()
	s.closed = true
	for _, cancel := range s.active {
		cancel()
	}
	s.activeMu.Unlock()
	if s.Work != nil {
		if err := s.Work.Close(ctx); err != nil {
			return err
		}
	}
	done := make(chan struct{})
	go func() { s.activeWG.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) Active() bool {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	return len(s.active) > 0
}

// AtReconfigurationPoint prevents new executions from entering between the
// persisted safe-point check and a change. A management tool can apply its
// change from the sole active run identified by exceptRun.
func (s *Service) AtReconfigurationPoint(ctx context.Context, exceptRun string, apply func() error) error {
	s.activeMu.Lock()
	_, own := s.active[exceptRun]
	if s.closed || s.reconfiguring || len(s.active) > 1 || (len(s.active) == 1 && !own) {
		s.activeMu.Unlock()
		return errors.New("reconfiguration requires an idle execution safe point")
	}
	s.reconfiguring = true
	s.activeMu.Unlock()
	defer func() {
		s.activeMu.Lock()
		s.reconfiguring = false
		s.activeMu.Unlock()
	}()
	if err := s.CheckReconfiguration(ctx, exceptRun); err != nil {
		return err
	}
	return apply()
}
