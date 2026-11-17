package ui

import (
	"context"
	"github.com/diasYuri/opera/internal/modules/execution"
)

type terminalExecutor struct{}

func (terminalExecutor) Start(context.Context, execution.Request) *execution.Execution {
	updates := make(chan execution.Update, 2)
	updates <- execution.Update{Event: &execution.Event{Type: "message.delta", Text: "PTY_RESPONSE_MARKER"}}
	updates <- execution.Update{Result: &execution.Result{ID: "pty-run", Text: "PTY_RESPONSE_MARKER", State: "completed"}}
	close(updates)
	return &execution.Execution{ID: "pty-run", Updates: updates}
}
func (terminalExecutor) FollowUp(string, string) error { return nil }
func (terminalExecutor) Steer(string, string) error    { return nil }
