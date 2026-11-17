package tui

import (
	"encoding/json"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func summarizeError(raw string) string {
	text := safeText(raw)
	// Prefer the API message over transport context; retain the raw error in /details.
	if start := strings.Index(text, "{"); start >= 0 {
		var body struct {
			Message string `json:"message"`
			Code    string `json:"code"`
			Error   *struct {
				Message string `json:"message"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		if json.NewDecoder(strings.NewReader(text[start:])).Decode(&body) == nil {
			if body.Error != nil {
				body.Message, body.Code = body.Error.Message, body.Error.Code
			}
			if body.Code == "model_not_found" {
				return "Model unavailable. Choose an available model with /model, then /retry."
			}
			if body.Message != "" {
				text = body.Message
			}
		}
	}
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return "Execution failed. Open /details for diagnostics."
	}
	return ansi.Truncate(text, 180, "…")
}

func stateLabel(state string) string {
	switch state {
	case "completed":
		return "Completed"
	case "failed":
		return "Failed"
	case "cancelled", "canceled":
		return "Cancelled · /retry to resume"
	case "awaiting_approval", "waiting_approval":
		return "Approval required"
	case "in_doubt":
		return "Outcome uncertain · reconcile before retry"
	default:
		return state
	}
}
