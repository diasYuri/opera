package execution

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/diasYuri/opera/internal/modules/accounting"
)

func (s *Service) Usage(ctx context.Context, scope, id, turn string) (accounting.Summary, error) {
	docs, err := s.Records.List(ctx, "usage")
	if err != nil {
		return accounting.Summary{}, err
	}
	rows := []accounting.Record{}
	for _, doc := range docs {
		var r accounting.Record
		if err := json.Unmarshal(doc.Data, &r); err != nil {
			return accounting.Summary{}, err
		}
		rows = append(rows, r)
	}
	selected := map[string]bool{}
	switch scope {
	case "workspace":
	case "run", "agent", "turn":
		metadata, err := s.Metadata(ctx, id)
		if err != nil {
			return accounting.Summary{}, err
		}
		if metadata.Workspace != s.Workspace {
			return accounting.Summary{}, errors.New("usage belongs to another workspace")
		}
		selected[id] = true
	case "session":
		docs, err := s.Records.List(ctx, "sessions")
		if err != nil {
			return accounting.Summary{}, err
		}
		for _, doc := range docs {
			var entry SessionEntry
			if err := json.Unmarshal(doc.Data, &entry); err != nil {
				return accounting.Summary{}, err
			}
			if entry.SessionID == id {
				selected[entry.ID] = true
			}
		}
	default:
		return accounting.Summary{}, errors.New("usage scope must be workspace, session, run, agent or turn")
	}
	if scope == "run" || scope == "session" {
		for changed := true; changed; {
			changed = false
			for _, r := range rows {
				if selected[r.ParentID] && !selected[r.RunID] {
					selected[r.RunID] = true
					changed = true
				}
			}
		}
	}
	filtered := []accounting.Record{}
	for _, r := range rows {
		if scope == "workspace" || selected[r.RunID] && (scope != "turn" || r.TurnID == turn) {
			filtered = append(filtered, r)
		}
	}
	return accounting.Summarize(filtered), nil
}
