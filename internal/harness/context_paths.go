package harness

import "sync"

// contextPathState is the explicit runtime-owned source shared by application
// metadata and the request hook. Updates remain visible after composition.
type contextPathState struct {
	mu    sync.RWMutex
	paths []string
}

func newContextPathState(paths []string) *contextPathState {
	state := &contextPathState{}
	state.Set(paths)
	return state
}

func (s *contextPathState) Current() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.paths...)
}

func (s *contextPathState) Set(paths []string) {
	s.mu.Lock()
	s.paths = append([]string(nil), paths...)
	s.mu.Unlock()
}
