package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/diasYuri/opera/internal/contextfiles"
	"github.com/diasYuri/y/pkg/ai"
)

type Update struct {
	Event  *Event
	Result *Result
	Error  error
}
type Execution struct {
	ID      string
	Updates <-chan Update
	cancel  context.CancelFunc
}

// NewExecution creates an execution stream for adapters that implement the
// Executor contract outside the app package. Cancel remains owned by the app
// so callers cannot replace it after the run starts.
func NewExecution(id string, updates <-chan Update, cancel context.CancelFunc) *Execution {
	return &Execution{ID: id, Updates: updates, cancel: cancel}
}

func (e *Execution) Cancel() {
	if e.cancel != nil {
		e.cancel()
	}
}

// Start owns the worker goroutine. UI commands only consume actions/events.
func (s *Service) Start(root context.Context, request Request) *Execution {
	if request.ID == "" {
		request.ID = rand.Text()
		request.newRun = true
	}
	ctx, cancel := context.WithCancel(root)
	updates := make(chan Update, 32)
	execution := NewExecution(request.ID, updates, cancel)
	go func() {
		defer close(updates)
		defer cancel()
		result, err := s.Execute(ctx, request, func(event Event) error {
			select {
			case updates <- Update{Event: &event}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		terminal := Update{Result: &result, Error: err}
		select {
		case updates <- terminal:
		default:
			select {
			case updates <- terminal:
			case <-ctx.Done():
			case <-root.Done():
			}
		}
	}()
	return execution
}

type Control struct {
	ID, RunID, Kind, Text, State, Error string
	Created                             time.Time
}

func (s *Service) FollowUp(id, text string) error {
	return s.sendControl(id, "follow_up", text, func(message ai.Message) error { return s.Runner.FollowUp(id, message) })
}
func (s *Service) FollowUpWithImages(id, text string, paths []string) error {
	return s.sendControlMessage(id, "follow_up", text, paths, func(message ai.Message) error { return s.Runner.FollowUp(id, message) })
}
func (s *Service) Steer(id, text string) error {
	return s.sendControl(id, "steer", text, func(message ai.Message) error { return s.Runner.Steer(id, message) })
}
func (s *Service) sendControl(id, kind, text string, apply func(ai.Message) error) error {
	return s.sendControlMessage(id, kind, text, nil, apply)
}
func (s *Service) sendControlMessage(id, kind, text string, paths []string, apply func(ai.Message) error) error {
	s.activeMu.Lock()
	if s.closed {
		s.activeMu.Unlock()
		return errors.New("execution service is closed")
	}
	s.activeWG.Add(1)
	s.activeMu.Unlock()
	defer s.activeWG.Done()

	s.controlMu.Lock()
	defer s.controlMu.Unlock()
	seq := uint64(0)
	if s.Records != nil {
		doc, err := s.Records.Get(context.Background(), "control-sequences", id)
		if err == nil {
			if err := json.Unmarshal(doc.Data, &seq); err != nil {
				return err
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		seq++
		if err := s.Records.Put(context.Background(), "control-sequences", id, seq); err != nil {
			return err
		}
	}
	key := id + ":" + fmt.Sprintf("%020d", seq) + ":" + rand.Text()
	content := []ai.ContentBlock{{Type: ai.ContentText, Text: text}}
	if len(paths) > 0 {
		if s.LoadImages == nil {
			return errors.New("image input is unavailable")
		}
		_, err := s.LoadImages(context.Background(), paths)
		if err != nil {
			return err
		}
		for _, path := range paths {
			content = append(content, contextfiles.ImageReference(path))
		}
	}
	message := ai.Message{ID: key, Role: ai.RoleUser, Content: content}
	if s.Records == nil {
		return apply(message)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	event := Control{ID: key, RunID: id, Kind: kind, Text: text, State: "requested", Created: time.Now().UTC()}
	if err := s.Records.Put(ctx, "controls", key, event); err != nil {
		return err
	}
	err := apply(message)
	event.State = "accepted"
	if err != nil {
		event.State = "rejected"
		event.Error = err.Error()
	}
	if saveErr := s.Records.Put(ctx, "controls", key, event); saveErr != nil {
		return saveErr
	}
	return err
}
