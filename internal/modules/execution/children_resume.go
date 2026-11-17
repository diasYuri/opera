package execution

import (
	"context"
	"github.com/diasYuri/y/pkg/extensions/subagents"
)

func (s *Service) resumeChildren(ctx context.Context, id string, request Request) (Result, bool, error) {
	children, err := s.Work.Children(ctx, id)
	if err != nil {
		return Result{ID: id, ExitCode: 1}, true, err
	}
	if len(children) == 0 {
		return Result{}, false, nil
	}
	for _, child := range children {
		if child.Status == subagents.StatusCompleted {
			continue
		}
		if child.Outcome.Approval != nil {
			decision := request.Approve
			if decision == "" {
				decision = request.Deny
			}
			if decision == "" || decision != child.Outcome.Approval.ID {
				return Result{ID: id, State: "waiting_children", ExitCode: 3, Approval: child.Outcome.Approval}, true, nil
			}
			outcome, e := s.Execute(ctx, Request{ID: child.ID, Approve: request.Approve, Deny: request.Deny}, nil)
			if e != nil {
				return outcome, true, e
			}
			if outcome.ExitCode != 0 {
				return Result{ID: id, State: "waiting_children", ExitCode: outcome.ExitCode, Approval: outcome.Approval}, true, nil
			}
			child.Outcome = outcome
			child.Status = subagents.StatusCompleted
			if err := s.Work.finish(child, outcome, nil); err != nil {
				return Result{ID: id, ExitCode: 1}, true, err
			}
			// A child approval never authorizes an operation in the parent.
			request.Approve = ""
			request.Deny = ""
		}
	}

	return Result{}, false, nil
}
