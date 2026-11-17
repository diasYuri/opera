package ui

import (
	"os"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textarea"
	"charm.land/lipgloss/v2"
)

func (m *model) applyThemePreference() {
	switch m.themeMode {
	case "dark", "midnight", "ocean", "forest", "sunset", "mono":
		m.dark = true
	case "light":
		m.dark = false
	default:
		m.dark = m.systemDark
	}
}

// Terminal backgrounds belong to the user. Semantic foreground tokens keep
// the composition legible on light, dark, transparent and colorless themes.
type palette struct {
	text, muted, accent, info, warning, error, success lipgloss.Style
	border, added, removed, userBubble                 lipgloss.Style
}

func (m model) theme() palette {
	plain := lipgloss.NewStyle()
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled {
		return palette{text: plain, muted: plain, accent: plain, info: plain, warning: plain, error: plain, success: plain, border: plain, added: plain, removed: plain, userBubble: plain}
	}
	// Each theme uses semantic colors so all UI components stay consistent.
	// Backgrounds remain terminal-owned; only the user's message bubble gets a
	// restrained background tint, matching the existing interface behavior.
	colors := map[string][8]string{
		"midnight": {"245", "141", "141", "221", "203", "120", "141", "238"},
		"ocean":    {"245", "81", "81", "220", "203", "120", "81", "24"},
		"forest":   {"245", "114", "114", "221", "203", "120", "114", "22"},
		"sunset":   {"245", "216", "216", "221", "203", "120", "216", "52"},
		"mono":     {"245", "250", "250", "252", "248", "252", "250", "236"},
	}
	if c, ok := colors[m.themeMode]; ok {
		return palette{
			text: plain, muted: plain.Foreground(lipgloss.Color(c[0])), accent: plain.Foreground(lipgloss.Color(c[1])),
			info: plain.Foreground(lipgloss.Color(c[2])), warning: plain.Foreground(lipgloss.Color(c[3])), error: plain.Foreground(lipgloss.Color(c[4])),
			success: plain.Foreground(lipgloss.Color(c[5])), border: plain.Foreground(lipgloss.Color(c[6])),
			added: plain.Foreground(lipgloss.Color(c[5])), removed: plain.Foreground(lipgloss.Color(c[4])),
			userBubble: plain.Foreground(lipgloss.Color("255")).Background(lipgloss.Color(c[7])),
		}
	}
	if m.dark {
		return palette{
			text: plain, muted: plain.Foreground(lipgloss.Color("248")), accent: plain.Foreground(lipgloss.Color("75")),
			info: plain.Foreground(lipgloss.Color("75")), warning: plain.Foreground(lipgloss.Color("215")), error: plain.Foreground(lipgloss.Color("203")),
			success: plain.Foreground(lipgloss.Color("42")), border: plain.Foreground(lipgloss.Color("75")),
			added: plain.Foreground(lipgloss.Color("42")), removed: plain.Foreground(lipgloss.Color("203")),
			userBubble: plain.Foreground(lipgloss.Color("255")).Background(lipgloss.Color("238")),
		}
	}
	return palette{
		text: plain, muted: plain.Foreground(lipgloss.Color("240")), accent: plain.Foreground(lipgloss.Color("25")),
		info: plain.Foreground(lipgloss.Color("25")), warning: plain.Foreground(lipgloss.Color("130")), error: plain.Foreground(lipgloss.Color("160")),
		success: plain.Foreground(lipgloss.Color("28")), border: plain.Foreground(lipgloss.Color("25")),
		added: plain.Foreground(lipgloss.Color("28")), removed: plain.Foreground(lipgloss.Color("160")),
		userBubble: plain.Foreground(lipgloss.Color("238")).Background(lipgloss.Color("254")),
	}
}

func (m *model) styleEditor() {
	t := m.theme()
	state := textarea.StyleState{Text: t.text, Placeholder: t.muted, Prompt: t.accent, EndOfBuffer: t.muted}
	styles := textarea.Styles{Focused: state, Blurred: state}
	// A steady virtual cursor keeps idle chat free of redraw timers.
	styles.Cursor.Blink = false
	if _, disabled := os.LookupEnv("NO_COLOR"); !disabled {
		styles.Cursor.Color = lipgloss.Color("25")
		if m.dark {
			styles.Cursor.Color = lipgloss.Color("75")
		}
	}
	m.editor.SetStyles(styles)
}

func (m model) modelDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.ShowDescription = true
	d.SetSpacing(0)
	d.SetHeight(2)
	t := m.theme()
	d.Styles.NormalTitle = t.text.PaddingLeft(2)
	d.Styles.SelectedTitle = t.accent.Bold(true).PaddingLeft(1).Border(lipgloss.NormalBorder(), false, false, false, true)
	d.Styles.DimmedTitle = t.muted.PaddingLeft(2)
	d.Styles.NormalDesc = t.muted.PaddingLeft(2)
	d.Styles.SelectedDesc = t.muted.PaddingLeft(2)
	d.Styles.DimmedDesc = t.muted.PaddingLeft(2)
	return d
}
