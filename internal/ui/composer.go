package ui

import "charm.land/bubbles/v2/textarea"

// composerModel owns the prompt editor and its submitted-prompt navigation.
// Other chat flows request focus or values through the editor control; history
// transitions stay here so the shell does not duplicate composer state.
type composerModel struct {
	editor       textarea.Model
	history      []string
	historyIndex int
}

func (c *composerModel) remember(text string) {
	c.history = append(c.history, text)
	c.historyIndex = len(c.history)
}

func (c *composerModel) navigateHistory(previous bool) {
	if len(c.history) == 0 {
		return
	}
	if previous {
		c.historyIndex = max(0, c.historyIndex-1)
	} else {
		c.historyIndex = min(len(c.history), c.historyIndex+1)
	}
	if c.historyIndex == len(c.history) {
		c.editor.Reset()
	} else {
		c.editor.SetValue(c.history[c.historyIndex])
	}
}
