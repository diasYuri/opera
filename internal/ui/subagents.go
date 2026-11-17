package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/diasYuri/opera/internal/modules/execution"
)

type SubagentItem struct {
	ID        string
	Name      string
	Status    string
	StartedAt time.Time
}

type SubagentSnapshotReader interface {
	SubagentSnapshot(context.Context, string) ([]SubagentItem, error)
}

type subagentSnapshotMsg struct {
	runID      string
	generation uint64
	items      []SubagentItem
	err        error
}

type subagentTickMsg struct {
	runID      string
	generation uint64
}

func (m *model) refreshSubagents(runID string) tea.Cmd {
	if runID == "" {
		return nil
	}
	reader, ok := m.service.(SubagentSnapshotReader)
	if !ok {
		return nil
	}
	m.subagentGeneration++
	generation := m.subagentGeneration
	m.subagentRunID = runID
	return readSubagentSnapshot(m.ctx, reader, runID, generation)
}

func readSubagentSnapshot(ctx context.Context, reader SubagentSnapshotReader, runID string, generation uint64) tea.Cmd {
	return func() tea.Msg {
		items, err := reader.SubagentSnapshot(ctx, runID)
		return subagentSnapshotMsg{runID: runID, generation: generation, items: items, err: err}
	}
}

func (m model) scheduleSubagentTick() tea.Cmd {
	runID, generation := m.subagentRunID, m.subagentGeneration
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return subagentTickMsg{runID: runID, generation: generation}
	})
}

func (m model) hasPendingSubagents() bool {
	for _, item := range m.subagents {
		if item.Status == "queued" || item.Status == "running" {
			return true
		}
	}
	return false
}

func (m model) isSubagentToolCompletion(event *execution.Event) bool {
	if event == nil || event.Type != "tool_completed" {
		return false
	}
	var payload eventPayload
	if json.Unmarshal(event.Data, &payload) != nil {
		return false
	}
	return strings.HasPrefix(payload.ToolResult.ToolName, "subagent_")
}

func (m model) subagentListHeight() int {
	if m.stream == nil {
		return 0
	}
	count := 0
	for _, item := range m.subagents {
		if item.Status == "running" {
			count++
		}
	}
	if count == 0 {
		return 0
	}
	visible := min(count, max(1, m.height-12))
	height := 1 + visible
	if visible < count {
		height++
	}
	return height
}

func (m model) subagentListView() string {
	if m.stream == nil {
		return ""
	}
	running := make([]SubagentItem, 0, len(m.subagents))
	for _, item := range m.subagents {
		if item.Status == "running" {
			running = append(running, item)
		}
	}
	if len(running) == 0 {
		return ""
	}
	sort.SliceStable(running, func(i, j int) bool {
		return running[i].StartedAt.Before(running[j].StartedAt)
	})
	t := m.theme()
	lines := []string{t.accent.Bold(true).Render(fmt.Sprintf("Subagents (%d running)", len(running)))}
	visible := min(len(running), max(1, m.height-12))
	for _, item := range running[:visible] {
		name := safeText(strings.TrimSpace(item.Name))
		if name == "" {
			name = "Subagent"
		}
		row := t.text.Render(name) + t.muted.Render(" - "+elapsed(item.StartedAt))
		lines = append(lines, ansi.Truncate(row, max(1, m.contentWidth()), "…"))
	}
	if visible < len(running) {
		lines = append(lines, t.muted.Render(fmt.Sprintf("… +%d more", len(running)-visible)))
	}
	return strings.Join(lines, "\n")
}

func elapsed(start time.Time) string {
	if start.IsZero() {
		return "0s"
	}
	d := time.Since(start).Truncate(time.Second)
	if d < 0 {
		d = 0
	}
	switch {
	case d >= time.Hour:
		return fmt.Sprintf("%dh %dm %ds", int(d/time.Hour), int(d/time.Minute)%60, int(d/time.Second)%60)
	case d >= time.Minute:
		return fmt.Sprintf("%dm %02ds", int(d/time.Minute), int(d/time.Second)%60)
	default:
		return fmt.Sprintf("%ds", int(d/time.Second))
	}
}
