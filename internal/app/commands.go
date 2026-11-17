package app

import "strings"

type Command struct{ Name, Usage, Description string }

var Commands = []Command{
	{"/new", "/new", "Start a clean conversation"},
	{"/clear", "/clear", "Clear the current conversation and start a new one"},
	{"/fork", "/fork", "Start a new session with a summary of this conversation"},
	{"/mode", "/mode [name]", "Choose the agent mode for future requests"},
	{"/plan", "/plan", "Toggle plan mode for future requests"},
	{"/login", "/login", "Connect a provider and choose an account model"},
	{"/usage", "/usage", "Inspect durable usage for this run and its children"},
	{"/tools", "/tools", "Inspect active tool schemas"},
	{"/skills", "/skills [name]", "List available Agent Skills or inspect one"},
	{"/mcp", "/mcp", "Inspect MCP configuration"},
	{"/memory", "/memory", "Inspect memory persistence"},
	{"/extensions", "/extensions [reload]", "Inspect generations or apply a supported idle transition"},
	{"/compact", "/compact", "Compact the previous completed checkpoint"},
	{"/retry", "/retry", "Explicitly resume the previous failed/interrupted execution"},
	{"/resume", "/resume", "Search saved sessions and resume a checkpoint"},
	{"/sessions", "/sessions", "Search saved sessions and continue a conversation"},
	{"/tree", "/tree", "Browse session branches, checkpoints and history"},
	{"/messages", "/messages", "Search user messages and jump to a match"},
	{"/tasks", "/tasks", "Inspect the persisted task hierarchy"},
	{"/agents", "/agents", "Inspect children and pending approvals"},
	{"/details", "/details", "Review an approval, run metadata or the latest error"},
	{"/model", "/model <provider> <model>", "Inspect or select provider/model at idle"},
	{"/thinking", "/thinking off|minimal|low|medium|high|xhigh|max", "Select supported reasoning level"},
	{"/context", "/context [path]", "List sources or add a UTF-8 file attachment"},
	{"/image", "/image <workspace-path> <message>", "Send a workspace image with this message"},
	{"/setting", "/setting", "Open appearance and workspace settings"},
	{"/settings", "/settings", "Edit model, thinking, context and workspace policy"},
	{"/help", "/help", "Show command help"},
	{"/approve", "/approve", "Approve the reviewed operation"},
	{"/deny", "/deny", "Deny the reviewed operation"},
	{"/steer", "/steer <message>", "Steer the active execution"},
}

func CommandHelp() string {
	var b strings.Builder
	for _, c := range Commands {
		b.WriteString(c.Usage + " — " + c.Description + "\n")
	}
	return b.String()
}
