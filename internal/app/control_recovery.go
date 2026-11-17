package app

import (
	"context"
	"encoding/json"
	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/ai"
)

// Stable message IDs reconcile the control journal with the last consistent
// checkpoint. Reopening for inspection never invokes this recovery path.
func (s *Service) recoverControls(ctx context.Context, id string, state *agent.Snapshot) error {
	if s.Records == nil {
		return nil
	}
	docs, err := s.Records.List(ctx, "controls")
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, list := range [][]ai.Message{state.Agent.Transcript, state.Agent.FollowUpQueue, state.Agent.PendingSteering} {
		for _, message := range list {
			seen[message.ID] = true
		}
	}
	added := false
	for _, doc := range docs {
		var control Control
		if err := json.Unmarshal(doc.Data, &control); err != nil {
			return err
		}
		if control.RunID != id || control.State == "rejected" || seen[control.ID] {
			continue
		}
		message := ai.Message{ID: control.ID, Role: ai.RoleUser, Content: []ai.ContentBlock{{Type: ai.ContentText, Text: control.Text}}}
		if control.Kind == "steer" {
			state.Agent.PendingSteering = append(state.Agent.PendingSteering, message)
		} else {
			state.Agent.FollowUpQueue = append(state.Agent.FollowUpQueue, message)
		}
		seen[control.ID] = true
		added = true
	}
	if added && state.Status == agent.RunCompleted {
		state.Status = agent.RunRecoverable
		state.Result = nil
		state.Owner = ""
		state.LeaseExpiresAt = state.UpdatedAt
		state.Agent.State = agent.StateIdle
	}
	return nil
}
