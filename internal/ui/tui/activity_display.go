package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/charmbracelet/x/ansi"
)

func (c conversationModel) shows(entry conversationEntry) bool {
	if c.display == "full" {
		return true
	}
	switch entry.kind {
	case conversationAssistant:
		return !entry.awaitingClassification && (!entry.intermediate || c.display == "auto")
	case conversationThinking:
		return false
	case conversationTool:
		return false
	case conversationVisual:
		return entry.visual.Kind != visualSkill && entry.visual.Kind != visualAgent && entry.visual.Kind != visualCompaction
	default:
		return true
	}
}

type conversationBlock struct {
	start, end int
	summary    string
}

// blocks projects history without discarding the entries used by Full and the
// activity inspector. Authored messages mark the boundaries between groups.
func (c conversationModel) blocks(yield func(conversationBlock) bool) {
	for i := 0; i < len(c.entries); {
		entry := c.entries[i]
		if c.display == "auto" && (entry.kind == conversationTool || entry.kind == conversationThinking) {
			end, count, failed, running := i, 0, 0, 0
			for ; end < len(c.entries); end++ {
				next := c.entries[end]
				if next.kind != entry.kind {
					if next.kind == conversationVisual && !c.shows(next) {
						continue
					}
					break
				}
				count++
				if next.failed {
					failed++
				}
				if next.state == "Running" {
					running++
				}
			}
			summary := "Thought"
			if entry.kind == conversationTool {
				summary = toolGroupSummary(count, failed, running)
			}
			if !yield(conversationBlock{start: i, end: end, summary: summary}) {
				return
			}
			i = end
			continue
		}
		if c.shows(entry) && !yield(conversationBlock{start: i, end: i + 1}) {
			return
		}
		i++
	}
}

func toolGroupSummary(count, failed, running int) string {
	noun := "tools"
	if count == 1 {
		noun = "tool"
	}
	summary := fmt.Sprintf("Ran %d %s", count, noun)
	if running > 0 {
		summary = fmt.Sprintf("%d %s · %d running", count, noun, running)
	}
	if failed > 0 {
		summary += fmt.Sprintf(" · %d failed", failed)
	}
	return summary
}

func (m model) renderConversationBlock(block conversationBlock) string {
	if block.summary == "" {
		return m.renderEntry(m.conversation.entries[block.start])
	}
	width := max(1, m.conversationWidth()-2)
	return "  " + m.theme().muted.Render(ansi.Truncate("› "+block.summary, width, "…"))
}

type activityGroupChoice struct {
	block  conversationBlock
	detail string
}

func (c activityGroupChoice) Title() string       { return c.block.summary }
func (c activityGroupChoice) Description() string { return c.detail }
func (c activityGroupChoice) FilterValue() string { return c.Title() + " " + c.Description() }

func (m *model) inspectActivity() {
	var items []list.Item
	for block := range m.conversation.blocks {
		if block.summary == "" {
			continue
		}
		entry := m.conversation.entries[block.start]
		detail := activityPreview(entry.text)
		if entry.kind == conversationTool {
			detail = activityPreview(entry.toolName)
		}
		items = append(items, activityGroupChoice{block: block, detail: detail})
	}
	slices.Reverse(items)
	if len(items) == 1 {
		m.openActivityGroup(items[0].(activityGroupChoice).block)
	} else if len(items) > 1 {
		m.openSelector("Activity details · newest first", items, m.modelDelegate())
	}
}

func (m *model) openActivityGroup(block conversationBlock) {
	var parts []string
	kind := m.conversation.entries[block.start].kind
	for _, entry := range m.conversation.entries[block.start:block.end] {
		if entry.kind != kind {
			continue
		}
		if kind == conversationTool {
			parts = append(parts, toolActivity(entry.state, entry.toolName)+"\n"+toolBody(entry, m.contentWidth()-4, true))
		} else {
			parts = append(parts, entry.text)
		}
	}
	m.openPanel(block.summary, strings.Join(parts, "\n\n"))
}

func activityPreview(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return ansi.Truncate(strings.Join(strings.Fields(safeText(line)), " "), 160, "…")
}

func toolActivity(state, name string) string {
	name = activityPreview(name)
	if name == "" {
		name = "tool"
	}
	return state + " · " + name
}

func (m *model) openActivitySetting() tea.Cmd {
	value := m.conversation.display
	field := huh.NewSelect[string]().Key("value").
		Options(
			huh.NewOption("Full · thinking, comments and tool details", "full"),
			huh.NewOption("Auto · comments and collapsed activity", "auto"),
			huh.NewOption("Hidden · responses and essential notices", "hidden"),
		).Value(&value).Height(4)
	form := huh.NewForm(huh.NewGroup(field)).WithShowHelp(true)
	return m.openForm("Activity display", form, "activity-display", "ui.activity", value)
}

func (m *model) activityCommand(text string) tea.Cmd {
	fields := strings.Fields(text)
	if len(fields) == 1 {
		return m.openActivitySetting()
	}
	if len(fields) == 2 {
		switch fields[1] {
		case "auto", "full", "hidden":
			return m.applySetting("ui.activity", fields[1])
		}
	}
	m.status = "Use /activity [full|auto|hidden]"
	return nil
}
