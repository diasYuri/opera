package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// chatKeyMap is the single source for the interactive chat's default key
// bindings and the labels shown in its footer. The configuration layer does
// not expose custom key bindings yet, so the defaults remain stable.
type chatKeyMap struct {
	submit, newline, complete     key.Binding
	pageUp, pageDown, top, bottom key.Binding
	details, cancel, quit, exit   key.Binding
	planMode                      key.Binding
	historyPrevious, historyNext  key.Binding
}

func defaultChatKeyMap() chatKeyMap {
	return chatKeyMap{
		submit:          key.NewBinding(key.WithKeys("enter"), key.WithHelp("Enter", "send")),
		newline:         key.NewBinding(key.WithKeys("shift+enter", "alt+enter", "ctrl+j"), key.WithHelp("Shift+Enter", "newline")),
		complete:        key.NewBinding(key.WithKeys("tab"), key.WithHelp("Tab", "complete")),
		pageUp:          key.NewBinding(key.WithKeys("pageup", "pgup"), key.WithHelp("PgUp", "scroll back")),
		pageDown:        key.NewBinding(key.WithKeys("pagedown", "pgdown"), key.WithHelp("PgDn", "scroll forward")),
		top:             key.NewBinding(key.WithKeys("ctrl+home"), key.WithHelp("Ctrl+Home", "oldest")),
		bottom:          key.NewBinding(key.WithKeys("ctrl+end"), key.WithHelp("Ctrl+End", "latest")),
		details:         key.NewBinding(key.WithKeys("ctrl+o"), key.WithHelp("Ctrl+O", "details")),
		planMode:        key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("Shift+Tab", "toggle plan mode")),
		cancel:          key.NewBinding(key.WithKeys("esc"), key.WithHelp("Esc", "back")),
		quit:            key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("Ctrl+C", "exit")),
		exit:            key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("Ctrl+D", "exit")),
		historyPrevious: key.NewBinding(key.WithKeys("up", "alt+up"), key.WithHelp("↑", "previous prompt")),
		historyNext:     key.NewBinding(key.WithKeys("down", "alt+down"), key.WithHelp("↓", "next prompt")),
	}
}

func (m model) keyBindings() chatKeyMap {
	if !m.keys.submit.Enabled() {
		return defaultChatKeyMap()
	}
	return m.keys
}

func keyLabel(binding key.Binding, fallback string) string {
	if !binding.Enabled() || binding.Help().Key == "" {
		return fallback
	}
	return binding.Help().Key
}

func keyMatches(message tea.KeyPressMsg, binding key.Binding) bool {
	return key.Matches(message, binding)
}
