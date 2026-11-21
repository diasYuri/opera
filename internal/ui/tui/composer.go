package tui

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/textarea"
)

// composerModel owns the prompt editor and its submitted-prompt navigation.
// Other chat flows request focus or values through the editor control; history
// transitions stay here so the shell does not duplicate composer state.
type composerModel struct {
	completionCancel context.CancelFunc
	editor           textarea.Model
	history          []string
	historyIndex     int
	historyDraft     string
}

func (c *composerModel) remember(text string) {
	c.history = append(c.history, text)
	c.historyIndex = len(c.history)
	c.historyDraft = ""
}

func (c *composerModel) navigateHistory(previous bool) {
	if len(c.history) == 0 {
		return
	}
	if previous {
		if c.historyIndex == len(c.history) {
			c.historyDraft = c.editor.Value()
		}
		c.historyIndex = max(0, c.historyIndex-1)
	} else {
		c.historyIndex = min(len(c.history), c.historyIndex+1)
	}
	if c.historyIndex == len(c.history) {
		c.editor.SetValue(c.historyDraft)
	} else {
		c.editor.SetValue(c.history[c.historyIndex])
	}
}

// shouldNavigateHistory reserves boundary arrows for prompt history only while
// editing a single-line draft. Multiline drafts keep both arrows for moving
// through their contents, including when the visible editor has reached its
// five-row limit and must scroll internally.
func (c composerModel) shouldNavigateHistory(previous bool) bool {
	if len(c.history) == 0 || strings.ContainsRune(c.editor.Value(), '\n') {
		return false
	}
	if previous {
		return c.editor.Line() == 0
	}
	lastLine := strings.Count(c.editor.Value(), "\n")
	return c.editor.Line() >= lastLine
}
