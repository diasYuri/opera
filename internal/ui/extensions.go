package ui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

type ExtensionContributionKind string

const (
	ExtensionHeader       ExtensionContributionKind = "header"
	ExtensionFooter       ExtensionContributionKind = "footer"
	ExtensionWidget       ExtensionContributionKind = "widget"
	ExtensionRenderer     ExtensionContributionKind = "renderer"
	ExtensionAutocomplete ExtensionContributionKind = "autocomplete"
)

// ExtensionContribution is a declarative UI contribution. Values are plain
// text and are always rendered with Opera's own styles and geometry limits.
type ExtensionContribution struct {
	ID, Kind, Target, Text, Detail, Trigger, Value string
}

// VisualExtensionProvider exposes a session-scoped snapshot of extension UI
// declarations. The provider owns loading; the TUI owns their presentation.
type VisualExtensionProvider interface {
	UIContributions(context.Context) ([]ExtensionContribution, error)
}

type visualExtensionsMsg struct {
	items []ExtensionContribution
	err   error
}

const maxExtensionContributions = 32

func normalizeExtensionContributions(items []ExtensionContribution) []ExtensionContribution {
	if len(items) > maxExtensionContributions {
		items = items[:maxExtensionContributions]
	}
	out := make([]ExtensionContribution, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		item.ID = strings.TrimSpace(safeText(item.ID))
		if item.ID == "" || seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		item.Text = contributionText(item.Text, 256)
		item.Detail = contributionText(item.Detail, 256)
		item.Trigger = contributionText(item.Trigger, 32)
		item.Value = contributionText(item.Value, 128)
		item.Target = strings.TrimSpace(safeText(item.Target))
		switch ExtensionContributionKind(item.Kind) {
		case ExtensionHeader, ExtensionFooter, ExtensionWidget:
			if item.Text == "" {
				continue
			}
		case ExtensionRenderer:
			if item.Text == "" || item.Target == "" {
				continue
			}
		case ExtensionAutocomplete:
			if item.Text == "" || item.Trigger == "" || item.Value == "" {
				continue
			}
		default:
			continue
		}
		out = append(out, item)
	}
	return out
}

func contributionText(value string, limit int) string {
	value = strings.ReplaceAll(safeText(value), "\n", " ")
	value = strings.Join(strings.Fields(value), " ")
	return ansi.Truncate(value, limit, "…")
}

func (m model) loadVisualContributions() tea.Cmd {
	provider, ok := m.service.(VisualExtensionProvider)
	if !ok {
		return nil
	}
	return func() tea.Msg {
		items, err := provider.UIContributions(m.ctx)
		return visualExtensionsMsg{items: items, err: err}
	}
}

func (m model) extensionHeader() string {
	return m.extensionLines(ExtensionHeader, 2)
}

func (m model) extensionFooter() string {
	return m.extensionLines(ExtensionFooter, 1)
}

func (m model) extensionWidgets() string {
	return m.extensionLines(ExtensionWidget, 3)
}

func (m model) extensionReservedHeight() int {
	return m.extensionLineCount(ExtensionHeader, 2) + m.extensionLineCount(ExtensionWidget, 3) + m.extensionLineCount(ExtensionFooter, 1)
}

func (m model) extensionLineCount(kind ExtensionContributionKind, limit int) int {
	count := 0
	for _, item := range m.extensionContributions {
		if ExtensionContributionKind(item.Kind) == kind {
			count++
			if count == limit {
				break
			}
		}
	}
	return count
}

func (m model) extensionLines(kind ExtensionContributionKind, maxLines int) string {
	var rows []string
	t := m.theme()
	for _, item := range m.extensionContributions {
		if ExtensionContributionKind(item.Kind) != kind || len(rows) >= maxLines {
			continue
		}
		prefix := ""
		if kind == ExtensionWidget {
			prefix = "  · "
		}
		rows = append(rows, t.info.Render(ansi.Truncate(prefix+item.Text, m.contentWidth(), "…")))
	}
	return strings.Join(rows, "\n")
}

func (m model) extensionRendererLabel(target, fallback string) string {
	for _, item := range m.extensionContributions {
		if item.Kind == string(ExtensionRenderer) && item.Target == target {
			return item.Text
		}
	}
	return fallback
}
