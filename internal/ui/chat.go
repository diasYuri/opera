// Package ui renders application events and emits application actions. Only
// Bubble Tea Update mutates the presentation state; commands perform all I/O.
package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/diasYuri/opera/internal/app"
	"github.com/diasYuri/opera/internal/skills"
)

type Executor interface {
	Start(context.Context, app.Request) *app.Execution
	FollowUp(string, string) error
	Steer(string, string) error
}
type ImageFollowUp interface {
	FollowUpWithImages(string, string, []string) error
}

type model struct {
	ctx          context.Context
	service      Executor
	editor       textarea.Model
	selector     list.Model
	panel        viewport.Model
	conversation viewport.Model
	spinner      spinner.Model
	keys         chatKeyMap
	approvalForm *huh.Form
	question     *questionFlow

	workspace              string
	workspaceRoot          string
	provider               string
	modelName              string
	panelTitle             string
	lastError              string
	runDetails             string
	text, status, previous string
	mode                   string
	imageProtocol          imageProtocol
	streamBuffer           *strings.Builder
	conversationEntries    []conversationEntry
	queuedMessages         []string
	attachments            []AttachmentPreview
	pastedImages           []string
	activeImagePaths       []string
	entryIndexes           map[string]int
	nextMessage            uint64
	runGeneration          uint64
	currentAssistantID     string
	todoTasks              []TaskItem
	todoRunID              string
	todoGeneration         uint64
	subagents              []SubagentItem
	subagentRunID          string
	subagentGeneration     uint64
	newConversationItems   bool
	toolDetailsExpanded    bool
	history                []string
	historyIndex           int
	committed              map[string]bool

	stream        *app.Execution
	approval      *app.Approval
	resume        app.Request
	lastSequence  uint64
	code          int
	width, height int

	retryPrevious                      bool
	loginPrompt                        *app.LoginPrompt
	login                              *loginFlow
	selectionKind                      string
	modelMenu                          bool
	selecting, panelOpen               bool
	formKind, formKey                  string
	formValue                          string
	formHeight                         int
	selectedProvider                   string
	selectedSession                    *SessionSummary
	selectedCheckpoint                 uint64
	treeSelection                      bool
	completions                        []completionItem
	skillCatalogCache                  skills.Catalog
	skillCatalogAt                     time.Time
	extensionContributions             []ExtensionContribution
	completionIndex                    int
	actionPending, quitting, followUp  bool
	dark, noMotion                     bool
	systemDark                         bool
	themeMode                          string
	activity                          activityModel
}

type started struct{ execution *app.Execution }
type frameMsg struct{ generation uint64 }
type taskSnapshotMsg struct {
	runID      string
	generation uint64
	snapshot   TaskSnapshot
	err        error
}
type updateMsg struct {
	update app.Update
	open   bool
}
type actionError struct{ err error }
type followUpResult struct {
	text string
	err  error
}
type clipboardImageMsg struct {
	data []byte
	err  error
}
type pastedImageSavedMsg struct {
	path string
	err  error
}
type choice string

func (c choice) Title() string       { return string(c) }
func (c choice) Description() string { return "Select for the next run" }
func (c choice) FilterValue() string { return string(c) }

type modeChoice struct{ mode app.Mode }

func (c modeChoice) Title() string       { return c.mode.Name }
func (c modeChoice) Description() string { return c.mode.Description }
func (c modeChoice) FilterValue() string { return c.mode.ID }

type choicesMsg struct {
	values []string
	err    error
}

// SessionInfo supplies display metadata without coupling the UI to a runtime.
type SessionInfo interface {
	SessionInfo() (workspace, provider, model string)
}

type ThemePreferenceProvider interface {
	ThemePreference() string
}

type ModelChooser interface {
	ModelChoices(context.Context) ([]string, error)
}
type ProviderModelChooser interface {
	ModelCatalogProvider
}
type panelMsg struct {
	text  string
	err   error
	title string
}
type Commander interface {
	Command(context.Context, string) (string, error)
}
type ShellExecutor interface {
	RunShell(context.Context, string) (string, error)
}
type SessionLifecycle interface {
	NewSession(context.Context) error
	ForkSession(context.Context, string) (string, error)
}
type sessionLifecycleMsg struct {
	command string
	id      string
	err     error
}
type shellResultMsg struct {
	command string
	output  string
	err     error
}

func Chat(ctx context.Context, service Executor, input io.Reader, output io.Writer, initial app.Request, noMotion ...bool) (int, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	m := newModel(ctx, service, initial, len(noMotion) > 0 && noMotion[0])
	m.imageProtocol = terminalImageProtocol(output)
	options := []tea.ProgramOption{tea.WithInput(input), tea.WithOutput(output), tea.WithContext(ctx), tea.WithFPS(20)}
	if _, set := os.LookupEnv("NO_COLOR"); set {
		options = append(options, tea.WithColorProfile(colorprofile.Ascii))
	}
	program := tea.NewProgram(m, options...)
	final, err := program.Run()
	if err != nil {
		if finished, ok := final.(model); ok {
			removePastedImages(append(finished.pastedImages, finished.activeImagePaths...), finished.workspaceRoot)
		}
		return 1, err
	}
	finished := final.(model)
	removePastedImages(append(finished.pastedImages, finished.activeImagePaths...), finished.workspaceRoot)
	return finished.code, nil
}
func (m model) Init() tea.Cmd {
	commands := []tea.Cmd{tea.RequestBackgroundColor, tea.Println(m.banner())}
	var background []tea.Cmd
	if cmd := m.loadVisualContributions(); cmd != nil {
		background = append(background, cmd)
	}
	if cmd := m.loadAttachmentPreviews(); cmd != nil {
		background = append(background, cmd)
	}
	if m.resume.ID != "" || m.resume.Prompt != "" {
		commands = append(commands, m.start(m.resume))
	} else {
		commands = append(commands, m.editor.Focus())
	}
	sequence := tea.Sequence(commands...)
	if len(background) == 0 {
		return sequence
	}
	return tea.Batch(sequence, tea.Batch(background...))
}

func (m model) start(request app.Request) tea.Cmd {
	return func() tea.Msg { return started{m.service.Start(m.ctx, request)} }
}
func next(execution *app.Execution) tea.Cmd {
	return func() tea.Msg { value, ok := <-execution.Updates; return updateMsg{value, ok} }
}

func (m model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case sessionsMsg:
		m.actionPending = false
		if msg.err != nil {
			m.lastError = msg.err.Error()
			m.status = summarizeError(msg.err.Error())
			return m, m.editor.Focus()
		}
		m.treeSelection = msg.tree
		items := make([]list.Item, 0, len(msg.values))
		for _, value := range msg.values {
			items = append(items, sessionChoice{value})
		}
		if len(items) == 0 {
			m.status = "No sessions are available for this workspace"
			return m, m.editor.Focus()
		}
		kind, title := "session-resume", "Resume a session"
		if msg.tree {
			kind, title = "session-tree", "Session branches and checkpoints"
		}
		if m.workspace != "" {
			title += " · " + m.workspace
		}
		m.openSelector(title, items, m.modelDelegate())
		m.selectionKind = kind
		return m, nil
	case sessionViewMsg:
		m.actionPending = false
		if msg.err != nil {
			m.lastError = msg.err.Error()
			m.status = summarizeError(msg.err.Error())
			return m, m.editor.Focus()
		}
		m.loadSessionView(msg.value)
		switch msg.mode {
		case "resume":
			m.previous = msg.value.Summary.ID
			m.retryPrevious = msg.value.Summary.State != "completed"
			m.resume = app.Request{ID: msg.value.Summary.ID}
			switch msg.value.Summary.State {
			case "in_doubt":
				m.retryPrevious = false
				m.status = "Outcome uncertain · inspect and reconcile before resuming"
			case "completed":
				m.status = "Session ready · the next message continues from this checkpoint"
			default:
				m.status = "Resuming saved checkpoint · browsing does not replay completed effects"
				m.actionPending = true
				return m, m.start(app.Request{ID: msg.value.Summary.ID})
			}
		case "branch-ready":
			m.previous = msg.value.Summary.ID
			m.retryPrevious = true
			m.resume = app.Request{ID: msg.value.Summary.ID}
			m.status = "Branch ready · enter a message to continue from the selected checkpoint"
			m.appendVisualEvent("branch:"+msg.value.Summary.ID, visualBranch, "Session branch", fmt.Sprintf("Created from checkpoint %d", msg.value.Summary.Checkpoint), "Ready")
			m.refreshConversation()
		default:
			m.previous = ""
			m.retryPrevious = false
			m.resume = app.Request{}
			m.status = fmt.Sprintf("Viewing %s · checkpoint %d", msg.value.Summary.SessionID, msg.value.Summary.Checkpoint)
		}
		return m, m.editor.Focus()
	case branchMsg:
		m.actionPending = false
		if msg.err != nil {
			m.lastError = msg.err.Error()
			m.status = summarizeError(msg.err.Error())
			return m, m.editor.Focus()
		}
		m.previous = msg.id
		return m, m.loadSession(msg.id, 0, "branch-ready")
	case modelCatalogMsg:
		m.actionPending = false
		m.status = "Ready"
		if msg.err != nil {
			m.lastError = msg.err.Error()
			m.status = summarizeError(msg.err.Error())
			if msg.provider == "openai-subscription" {
				m.status += " · use /login to connect the account"
			} else {
				m.status += " · check provider credentials and endpoint settings"
			}
			return m, m.editor.Focus()
		}
		items := make([]list.Item, 0, len(msg.values))
		for _, value := range msg.values {
			items = append(items, modelChoice{value})
		}
		if len(items) == 0 {
			m.status = "No models available · connect a provider with /login or check credentials"
			return m, m.editor.Focus()
		}
		delegate := m.modelDelegate()
		delegate.ShowDescription = false
		delegate.SetHeight(1)
		m.openSelector("", items, delegate)
		m.modelMenu = true
		m.selectionKind = "model-menu"
		m.selector.SetSize(max(1, m.contentWidth()-4), min(m.viewerHeight(), len(items)))
		m.resize()
		return m, nil
	case modelSelectionMsg:
		m.actionPending = false
		if msg.err != nil {
			m.lastError = msg.err.Error()
			m.status = summarizeError(msg.err.Error())
			return m, m.editor.Focus()
		}
		m.refreshContext()
		m.lastError = ""
		m.status = "Model selected"
		return m, m.editor.Focus()
	case settingResultMsg:
		m.actionPending = false
		if msg.err != nil {
			m.lastError = msg.err.Error()
			m.status = summarizeError(msg.err.Error())
			m.resize()
			return m, m.editor.Focus()
		}
		m.refreshContext()
		if msg.key == "ui.theme" {
			m.applyThemePreference()
			m.styleEditor()
			m.refreshConversation()
		}
		m.status = msg.message
		m.lastError = ""
		m.resize()
		if msg.key == "context.add" || msg.key == "context.remove" {
			return m, tea.Batch(m.editor.Focus(), m.loadAttachmentPreviews())
		}
		return m, m.editor.Focus()
	case settingsMsg:
		m.actionPending = false
		items := make([]list.Item, 0, len(msg.values))
		for _, value := range msg.values {
			items = append(items, settingChoice{value})
		}
		m.openSelector("Workspace settings", items, m.modelDelegate())
		m.selectionKind = "settings"
		return m, nil
	case visualExtensionsMsg:
		if msg.err != nil {
			m.status = "Some extension UI contributions could not be loaded"
			return m, nil
		}
		m.extensionContributions = normalizeExtensionContributions(msg.items)
		m.refreshCompletions()
		m.resize()
		return m, nil
	case attachmentPreviewsMsg:
		if msg.err != nil {
			m.attachments = nil
			m.status = "Attachment preview metadata is unavailable"
			return m, nil
		}
		m.attachments = msg.items
		m.prepareAttachmentPreviews()
		m.resize()
		return m, nil
	case loginEvent:
		return m.updateLogin(msg)
	case choicesMsg:
		m.actionPending = false
		if msg.err != nil {
			m.status = summarizeError(msg.err.Error())
			m.lastError = msg.err.Error()
			return m, m.editor.Focus()
		}
		items := make([]list.Item, 0, len(msg.values))
		for _, v := range msg.values {
			items = append(items, choice(v))
		}
		if len(items) == 0 {
			m.status = "No models are available for this account · /login to reconnect"
			return m, m.editor.Focus()
		}
		m.openSelector("Models", items, m.modelDelegate())
		m.selectionKind = "model"

		return m, nil
	case panelMsg:
		m.actionPending = false
		m.refreshContext()
		title := msg.title
		if title == "" {
			title = "Session details"
		}
		m.openPanel(title, msg.text)
		if msg.err != nil {
			m.lastError = msg.err.Error()
			m.openPanel("Error details", m.lastError)
		}
		return m, nil
	case shellResultMsg:
		m.actionPending = false
		m.nextMessage++
		id := fmt.Sprintf("shell:%d", m.nextMessage)
		output := msg.output
		state := "Completed"
		if msg.err != nil {
			state = "Failed"
			m.lastError = msg.err.Error()
			m.status = summarizeError(msg.err.Error())
			if output != "" {
				output += "\n"
			}
			output += "Command failed: " + msg.err.Error()
		} else {
			m.lastError = ""
			m.status = "Ready"
		}
		m.appendEntry(conversationEntry{id: id, kind: conversationTool, toolID: id, toolName: "bash", arguments: msg.command, text: output, state: state, failed: msg.err != nil})
		m.refreshConversation()
		m.resize()
		return m, m.editor.Focus()
	case sessionLifecycleMsg:
		m.actionPending = false
		if msg.err != nil {
			m.lastError = msg.err.Error()
			m.status = summarizeError(msg.err.Error())
			return m, m.editor.Focus()
		}
		m.clearConversation()
		if msg.command == "/fork" {
			m.previous = msg.id
			m.resume = app.Request{ID: msg.id}
			m.retryPrevious = true
			return m, m.loadSession(msg.id, 0, "branch-ready")
		}
		m.status = "Started a new session"
		return m, m.editor.Focus()
	case started:
		m.actionPending = false
		m.stream = msg.execution
		m.todoTasks = nil
		m.todoRunID = msg.execution.ID
		m.subagents = nil
		m.subagentRunID = msg.execution.ID
		m.refreshContext()
		m.text = ""
		m.streamBuffer = &strings.Builder{}
		m.lastSequence = 0
		m.activity.Start()
		m.lastError = ""
		m.status = "Running"
		m.runGeneration++
		m.currentAssistantID = fmt.Sprintf("assistant:%s:%d", msg.execution.ID, m.runGeneration)
		m.appendEntry(conversationEntry{id: m.currentAssistantID, kind: conversationAssistant, state: "Opera"})
		m.resize()
		commands := []tea.Cmd{next(m.stream), m.animate()}
		if cmd := m.loadTaskSnapshot(m.stream.ID); cmd != nil {
			commands = append(commands, cmd)
		}
		if cmd := m.refreshSubagents(m.stream.ID); cmd != nil {
			commands = append(commands, cmd)
		}
		return m, tea.Batch(commands...)
	case taskSnapshotMsg:
		if msg.runID != m.todoRunID || msg.generation != m.todoGeneration {
			return m, nil
		}
		if msg.err == nil {
			m.todoTasks = msg.snapshot.Tasks
			m.resize()
		}
		return m, nil
	case subagentSnapshotMsg:
		if msg.runID != m.subagentRunID || msg.generation != m.subagentGeneration {
			return m, nil
		}
		if msg.err == nil {
			m.subagents = msg.items
			m.resize()
		}
		if msg.err != nil || m.hasPendingSubagents() {
			return m, m.scheduleSubagentTick()
		}
		return m, nil
	case subagentTickMsg:
		if msg.runID != m.subagentRunID || msg.generation != m.subagentGeneration {
			return m, nil
		}
		reader, ok := m.service.(SubagentSnapshotReader)
		if !ok {
			return m, nil
		}
		return m, readSubagentSnapshot(m.ctx, reader, msg.runID, msg.generation)
	case frameMsg:
		return m.updateFrame(msg)
	case actionError:
		if msg.err != nil {
			m.status = summarizeError(msg.err.Error())
			m.lastError = msg.err.Error()
		}
		return m, nil
	case clipboardImageMsg:
		if msg.err != nil {
			m.status = "Clipboard has no supported image"
			return m, nil
		}
		return m, savePastedImage(m.workspaceRoot, msg.data)
	case pastedImageSavedMsg:
		if msg.err != nil {
			m.lastError = msg.err.Error()
			m.status = "Could not attach clipboard image"
			return m, nil
		}
		m.pastedImages = append(m.pastedImages, msg.path)
		m.status = fmt.Sprintf("Image attached · %d · enter a message to send", len(m.pastedImages))
		m.resize()
		return m, nil
	case followUpResult:
		if msg.err != nil {
			m.status = summarizeError(msg.err.Error())
			m.lastError = msg.err.Error()
			return m, nil
		}
		m.followUp = true
		m.queuedMessages = append(m.queuedMessages, msg.text)
		m.resize()
		return m, nil
	case updateMsg:
		if !msg.open {
			if m.stream != nil {
				m.previous = m.stream.ID
				m.retryPrevious = true
				m.stream = nil
				m.subagentGeneration++
				m.subagents = nil
				m.code = 130
				m.status = "Execution interrupted; resume the saved run explicitly"
			}
			return m, nil
		}
		if msg.update.Event != nil {
			event := msg.update.Event
			m.openUserQuestion(event)
			if m.stream != nil && event.RunID != "" && event.RunID != m.stream.ID {
				m.updateConversationEvent(event)
				return m, next(m.stream)
			}
			if event.Sequence > 0 && event.Sequence <= m.lastSequence {
				return m, next(m.stream)
			}
			m.lastSequence = event.Sequence
			m.updateConversationEvent(event)
			commands := []tea.Cmd{next(m.stream)}
			if event.RunID == m.stream.ID && m.isTaskToolCompletion(event) {
				if cmd := m.loadTaskSnapshot(m.stream.ID); cmd != nil {
					commands = append(commands, cmd)
				}
			}
			if event.RunID == m.stream.ID && m.isSubagentToolCompletion(event) {
				if cmd := m.refreshSubagents(m.stream.ID); cmd != nil {
					commands = append(commands, cmd)
				}
			}
			return m, tea.Batch(commands...)
		}
		if msg.update.Result != nil {
			result := msg.update.Result
			removePastedImages(m.activeImagePaths, m.workspaceRoot)
			m.activeImagePaths = nil
			m.stream = nil
			m.subagentGeneration++
			m.subagents = nil
			m.activity.Stop()
			if result.State != "" {
				m.previous = result.ID
				m.retryPrevious = result.Approval == nil && (result.State == "failed" && result.ExitCode == 1 || (result.State == "cancelled" || result.State == "canceled") && result.ExitCode == 130)
			}
			m.code = result.ExitCode
			m.approval = result.Approval
			m.resume = app.Request{ID: result.ID}
			m.status = stateLabel(result.State)
			m.runDetails = fmt.Sprintf("Run: %s\nState: %s\nCheckpoint: %d", result.ID, result.State, result.Checkpoint)
			if result.State == "in_doubt" || result.ExitCode == 4 {
				m.status = "Outcome uncertain · reconcile before retry"
				m.runDetails += fmt.Sprintf("\n\nInspect with: opera sessions operations %s\nAfter observing the effect, reconcile with: opera reconcile %s <operation-key> <observed-result>", result.ID, result.ID)
			}
			text := m.text
			if text == "" {
				text = result.Text
			}
			m.text = ""
			m.streamBuffer = nil
			commands := []tea.Cmd{}
			if m.committed == nil {
				m.committed = map[string]bool{}
			}
			identity := fmt.Sprintf("%s:%d", result.ID, result.Checkpoint)
			if !m.committed[identity] {
				m.committed[identity] = true
				if text != "" {
					id := m.currentAssistantID
					if id == "" {
						id = fmt.Sprintf("assistant:%s:%d", result.ID, result.Checkpoint)
					}
					m.appendEntry(conversationEntry{id: id, kind: conversationAssistant, text: text, state: "Opera"})
				}
				if msg.update.Error != nil {
					m.appendEntry(conversationEntry{id: fmt.Sprintf("error:%s:%d", result.ID, result.Checkpoint), kind: conversationError, text: msg.update.Error.Error()})
				}
			}
			if msg.update.Error != nil {
				m.lastError = msg.update.Error.Error()
				if result.ExitCode != 4 {
					m.status = "Failed"
				}
			}

			if m.approval != nil {
				m.status = "Approval required"
				m.appendEntry(conversationEntry{id: "approval:" + m.approval.ID, kind: conversationSystem, text: "Approval required for " + m.approval.Tool + ". Review the operation, then allow or deny it."})
				if cmd := m.openApproval(m.approval); cmd != nil {
					commands = append(commands, cmd)
				}
			}
			m.resize()
			if m.quitting {
				return m, tea.Sequence(append(commands, tea.Quit)...)
			}
			if len(m.queuedMessages) == 0 {
				m.followUp = false
			} else if result.ExitCode == 0 {
				m.commitQueuedMessages()
				m.followUp = false
				commands = append(commands, m.start(app.Request{ContinueFrom: m.previous}))
			}
			return m, tea.Sequence(commands...)
		}
	case tea.BackgroundColorMsg:
		m.systemDark = msg.IsDark()
		m.applyThemePreference()
		m.styleEditor()
		if m.approvalForm != nil {
			m.approvalForm.WithTheme(approvalTheme(m.dark))
		}
		m.refreshConversation()
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil
	case tea.KeyPressMsg:
		if msg.String() == "super+v" || msg.String() == "meta+v" {
			return m, m.readClipboardImage()
		}
		keys := m.keyBindings()
		if m.login != nil && m.loginPrompt != nil {
			return m.updateLoginPrompt(message, msg)
		}
		if m.login != nil {
			if keyMatches(msg, keys.cancel) || keyMatches(msg, keys.quit) {
				m.login.cancel()
				m.status = "Cancelling login…"
			}
			return m, nil
		}
		if nextModel, cmd, handled := m.updateQuestionKey(msg); handled {
			return nextModel, cmd
		}
		if m.selecting {
			switch {
			case keyMatches(msg, keys.cancel):
				m.selecting = false
				m.modelMenu = false
				m.selectionKind = ""
				m.resize()
				return m, m.editor.Focus()
			case keyMatches(msg, keys.quit):
				return m, tea.Quit
			case keyMatches(msg, keys.submit):
				return m.selectCurrent()
			}
			var cmd tea.Cmd
			m.selector, cmd = m.selector.Update(message)
			return m, cmd
		}
		if m.panelOpen {
			if keyMatches(msg, keys.quit) {
				if m.approvalForm != nil {
					m.approvalForm = nil
					m.panelOpen = false
					m.formKind = ""
					m.formHeight = 0
					m.resize()
					return m, m.editor.Focus()
				}
				if m.stream != nil {
					m.panelOpen = false
					m.status = "Cancelling execution…"
					m.editor.Focus()
					return m, func() tea.Msg { m.stream.Cancel(); return nil }
				}
				return m, tea.Quit
			}
			if keyMatches(msg, keys.cancel) {
				m.panelOpen = false
				m.approvalForm = nil
				m.formKind = ""
				m.formHeight = 0
				m.resize()
				return m, m.editor.Focus()
			}
			if keyMatches(msg, keys.pageUp) {
				m.panel.PageUp()
				return m, nil
			}
			if keyMatches(msg, keys.pageDown) {
				m.panel.PageDown()
				return m, nil
			}
			if m.approvalForm != nil {
				form := m.approvalForm
				_, cmd := form.Update(message)
				if form.State == huh.StateCompleted {
					return m.finishForm()
				}
				if form.State == huh.StateAborted {
					m.approvalForm = nil
					m.panelOpen = false
					m.formKind = ""
					m.formHeight = 0
					m.resize()
					return m, m.editor.Focus()
				}
				return m, cmd
			}
			var cmd tea.Cmd
			m.panel, cmd = m.panel.Update(message)
			return m, cmd
		}
		if keyMatches(msg, keys.submit) && strings.TrimSpace(m.editor.Value()) == "/login" {
			m.completions = nil
		}
		if len(m.completions) > 0 {
			switch {
			case keyMatches(msg, keys.complete):
				m.acceptCompletion()
				return m, nil
			case keyMatches(msg, keys.submit):
				completion := m.completions[m.completionIndex]
				if completion.start == 0 && strings.HasPrefix(completion.replacement, "/") {
					m.acceptCompletion()
					// Keep processing Enter below so the selected slash command runs now.
				} else {
					m.acceptCompletion()
					return m, nil
				}
			case msg.String() == "up":
				m.completionIndex = (m.completionIndex + len(m.completions) - 1) % len(m.completions)
				return m, nil
			case msg.String() == "down":
				m.completionIndex = (m.completionIndex + 1) % len(m.completions)
				return m, nil
			case keyMatches(msg, keys.cancel):
				m.completions = nil
				return m, nil
			}
		}
		switch {
		case keyMatches(msg, keys.planMode):
			if m.stream != nil || m.approval != nil {
				m.status = "Resolve the active execution before changing modes"
				return m, nil
			}
			m.togglePlanMode()
			return m, nil
		case keyMatches(msg, keys.details):
			m.toolDetailsExpanded = !m.toolDetailsExpanded
			m.refreshConversation()
			return m, nil
		case keyMatches(msg, keys.pageUp):
			m.conversation.PageUp()
			m.newConversationItems = !m.conversation.AtBottom()
			return m, nil
		case keyMatches(msg, keys.pageDown):
			m.conversation.PageDown()
			m.newConversationItems = !m.conversation.AtBottom()
			return m, nil
		case keyMatches(msg, keys.top):
			m.conversation.GotoTop()
			m.newConversationItems = true
			return m, nil
		case keyMatches(msg, keys.bottom):
			m.conversation.GotoBottom()
			m.newConversationItems = false
			return m, nil
		case keyMatches(msg, keys.historyPrevious), keyMatches(msg, keys.historyNext):
			if len(m.history) == 0 {
				return m, nil
			}
			if keyMatches(msg, keys.historyPrevious) {
				m.historyIndex = max(0, m.historyIndex-1)
			} else {
				m.historyIndex = min(len(m.history), m.historyIndex+1)
			}
			if m.historyIndex == len(m.history) {
				m.editor.Reset()
			} else {
				m.editor.SetValue(m.history[m.historyIndex])
			}
			m.sizeEditor()
			return m, nil
		case keyMatches(msg, keys.quit):
			if m.stream != nil {
				m.status = "Cancelling execution…"
				return m, func() tea.Msg { m.stream.Cancel(); return nil }
			}
			return m, tea.Quit
		case keyMatches(msg, keys.exit):
			if m.editor.Value() == "" {
				if m.stream != nil {
					m.quitting = true
					m.status = "Cancelling execution…"
					return m, func() tea.Msg { m.stream.Cancel(); return nil }
				}
				return m, tea.Quit
			}
		case keyMatches(msg, keys.submit):
			if m.actionPending {
				return m, nil
			}
			text := strings.TrimSpace(m.editor.Value())
			if text == "" {
				return m, nil
			}
			m.editor.Reset()
			m.sizeEditor()
			m.history = append(m.history, text)
			m.historyIndex = len(m.history)
			commandName, _, _ := strings.Cut(text, " ")
			if strings.HasPrefix(text, "!") {
				command := strings.TrimSpace(strings.TrimPrefix(text, "!"))
				if command == "" {
					m.status = "Usage: !<bash command>"
					return m, m.editor.Focus()
				}
				if m.stream != nil || m.approval != nil {
					m.status = "Resolve the active execution before running a shell command"
					return m, m.editor.Focus()
				}
				shell, ok := m.service.(ShellExecutor)
				if !ok {
					m.status = "Shell commands are unavailable in this session"
					return m, m.editor.Focus()
				}
				m.appendUser(text, "You")
				m.actionPending = true
				m.lastError = ""
				m.status = "Running shell command…"
				return m, func() tea.Msg {
					output, err := shell.RunShell(m.ctx, command)
					return shellResultMsg{command: command, output: output, err: err}
				}
			}
			if commandName == "/new" || commandName == "/clear" || commandName == "/fork" {
				if text != commandName {
					m.status = "Use the command without arguments"
					return m, nil
				}
				if m.stream != nil || m.approval != nil || m.actionPending {
					m.status = "Resolve the active action before changing sessions"
					return m, nil
				}
				lifecycle, ok := m.service.(SessionLifecycle)
				if !ok {
					m.status = "Session commands are unavailable"
					return m, nil
				}
				id := m.previous
				if commandName == "/fork" && id == "" {
					m.status = "There is no completed session to fork"
					return m, nil
				}
				m.actionPending = true
				if commandName == "/fork" {
					m.status = "Summarizing the current session…"
					return m, func() tea.Msg {
						branch, err := lifecycle.ForkSession(m.ctx, id)
						return sessionLifecycleMsg{command: commandName, id: branch, err: err}
					}
				}
				return m, func() tea.Msg { return sessionLifecycleMsg{command: commandName, err: lifecycle.NewSession(m.ctx)} }
			}
			if commandName == "/plan" {
				if text != "/plan" {
					m.status = "Use /plan to toggle planning mode"
					return m, nil
				}
				if m.stream != nil || m.approval != nil {
					m.status = "Resolve the active execution before changing modes"
					return m, nil
				}
				m.togglePlanMode()
				return m, nil
			}
			if commandName == "/mode" {
				if m.stream != nil || m.approval != nil {
					m.status = "Resolve the active execution before changing modes"
					return m, nil
				}
				fields := strings.Fields(text)
				if len(fields) == 1 {
					m.openModePicker()
					return m, nil
				}
				if len(fields) != 2 {
					m.status = "Use /mode [name]"
					return m, nil
				}
				if err := m.selectMode(fields[1]); err != nil {
					m.status = err.Error()
				}
				return m, nil
			}
			if commandName == "/login" {
				if m.stream != nil || m.approval != nil {
					m.status = "Resolve the active execution before signing in"
					return m, nil
				}
				if text != "/login" {
					m.status = "Use /login, then choose a provider"
					return m, nil
				}
				m.openLoginProviders()
				return m, nil
			}
			if commandName == "/resume" || commandName == "/sessions" || commandName == "/tree" {
				if m.stream != nil || m.approval != nil {
					m.status = "Resolve the active execution before browsing saved sessions"
					return m, nil
				}
				if text != commandName {
					m.status = "Use the command without arguments; then filter the session list"
					return m, nil
				}
				return m, m.openSessionPicker(commandName == "/tree")
			}
			if commandName == "/messages" {
				m.openMessagePicker()
				return m, nil
			}
			if commandName == "/retry" {
				if m.stream != nil || m.approval != nil || m.previous == "" || !m.retryPrevious {
					m.status = "Retry requires a settled, retryable execution with no pending approval"
					return m, nil
				}
				m.actionPending = true
				return m, m.start(app.Request{ID: m.previous})
			}
			if commandName == "/usage" || commandName == "/compact" || commandName == "/tasks" || commandName == "/agents" {
				if m.previous == "" && m.stream == nil {
					m.status = "No session yet"
					return m, nil
				}
				if m.stream != nil && commandName == "/compact" {
					m.status = "Compaction requires idle execution"
					return m, nil
				}
				if commander, ok := m.service.(Commander); ok {
					m.actionPending = true
					id := m.previous
					if m.stream != nil {
						id = m.stream.ID
					}
					return m, func() tea.Msg {
						v, e := commander.Command(m.ctx, commandName+" "+id)
						return panelMsg{text: v, err: e, title: strings.TrimPrefix(commandName, "/")}
					}
				}
			}

			if commandName == "/details" {
				if m.approval != nil {
					m.openPanel("Review operation", approvalText(m.approval))
				} else if m.lastError != "" {
					m.openPanel("Error details", m.runDetails+"\n\n"+m.lastError)
				} else if m.runDetails != "" {
					m.openPanel("Run details", m.runDetails)
				} else {
					m.status = "No execution details yet"
				}
				return m, nil
			}
			if commandName == "/settings" && text == "/settings" || commandName == "/model" && text == "/model" || commandName == "/thinking" && text == "/thinking" || commandName == "/context" && text == "/context" {
				if m.stream != nil || m.approval != nil {
					m.status = "Resolve the active execution before changing settings"
					return m, nil
				}
				if commandName == "/settings" {
					return m, m.openSettingsPicker()
				}
				if commandName == "/model" {
					return m, m.openModelPicker()
				}
				manager, ok := m.service.(SettingsManager)
				if ok {
					for _, setting := range manager.Settings() {
						if setting.Key == commandName[1:] {
							return m, m.openSetting(setting)
						}
					}
				}
				return m, nil
			}
			if commandName == "/tools" || commandName == "/mcp" || commandName == "/extensions" || commandName == "/memory" || commandName == "/help" || commandName == "/context" || commandName == "/model" || commandName == "/thinking" {
				if m.stream != nil || m.approval != nil {
					m.status = "Resolve the active execution before changing settings"
					return m, nil
				}
				if commander, ok := m.service.(Commander); ok {
					m.actionPending = true
					return m, func() tea.Msg {
						v, e := commander.Command(m.ctx, text)
						return panelMsg{text: v, err: e, title: strings.TrimPrefix(commandName, "/")}
					}
				}
				return m, func() tea.Msg { return panelMsg{text: app.CommandHelp()} }
			}
			if strings.HasPrefix(text, "/") && commandName != "/approve" && commandName != "/deny" && commandName != "/steer" && commandName != "/image" {
				m.status = "Unknown command; use /help"
				return m, nil
			}
			if text == "/approve" || text == "/deny" {
				if m.approval == nil {
					m.status = "No approval is pending"
					return m, nil
				}
				return m, m.resolveApproval(text == "/approve")
			}
			if commandName == "/steer" && (m.stream == nil || len(strings.Fields(text)) < 2) {
				m.status = "Steering requires an active execution and a message"
				return m, nil
			}
			if strings.HasPrefix(text, "/image ") {
				path, prompt, ok := parseImageCommand(strings.TrimPrefix(text, "/image "))
				if !ok {
					m.status = "Usage: /image <workspace-path> <message>"
					return m, nil
				}
				shown := "🖼 " + path + "\n" + prompt
				if m.stream != nil {
					imageService, supported := m.service.(ImageFollowUp)
					if !supported {
						m.status = "Image follow-up is unavailable"
						return m, nil
					}
					return m, func() tea.Msg {
						return followUpResult{text: shown, err: imageService.FollowUpWithImages(m.stream.ID, prompt, []string{path})}
					}
				}
				if m.approval != nil {
					m.status = "Resolve the pending operation with /approve or /deny first"
					return m, nil
				}
				request := m.chatRequest(prompt)
				request.ImagePaths = []string{path}
				m.appendUser(shown, "You")
				m.actionPending = true
				return m, m.start(request)
			}
			if strings.HasPrefix(text, "/steer ") && m.stream != nil {
				return m, func() tea.Msg { return actionError{m.service.Steer(m.stream.ID, strings.TrimPrefix(text, "/steer "))} }
			}
			if m.stream != nil {
				if len(m.pastedImages) > 0 {
					imageService, supported := m.service.(ImageFollowUp)
					if !supported {
						m.status = "Image follow-up is unavailable"
						return m, nil
					}
					paths := append([]string(nil), m.pastedImages...)
					m.activeImagePaths = append(m.activeImagePaths, paths...)
					m.pastedImages = nil
					shown := fmt.Sprintf("🖼 ×%d\n%s", len(paths), text)
					return m, func() tea.Msg {
						return followUpResult{text: shown, err: imageService.FollowUpWithImages(m.stream.ID, text, paths)}
					}
				}
				return m, func() tea.Msg { return followUpResult{text: text, err: m.service.FollowUp(m.stream.ID, text)} }
			}
			if m.approval != nil {
				m.status = "Resolve the pending operation with /approve or /deny first"
				return m, nil
			}
			request := m.chatRequest(text)
			request.ImagePaths = append([]string(nil), m.pastedImages...)
			m.activeImagePaths = append(m.activeImagePaths, m.pastedImages...)
			m.pastedImages = nil
			shown := text
			if len(request.ImagePaths) > 0 {
				shown = fmt.Sprintf("🖼 ×%d\n%s", len(request.ImagePaths), text)
			}
			m.appendUser(shown, "You")
			m.actionPending = true
			return m, m.start(request)
		}
	}
	var command tea.Cmd
	m.editor, command = m.editor.Update(message)
	m.sizeEditor()
	m.refreshCompletions()
	return m, command
}

func (m *model) acceptCompletion() {
	if len(m.completions) == 0 {
		return
	}
	completion := m.completions[m.completionIndex]
	applyCompletion(&m.editor, completion)
	m.completions = nil
	m.completionIndex = 0
	m.sizeEditor()
}

func (m *model) togglePlanMode() {
	if m.mode == app.ModePlan {
		m.mode = app.ModeDefault
	} else {
		m.mode = app.ModePlan
	}
	m.status = "Ready"
	m.resize()
}

func (m *model) selectMode(id string) error {
	mode, err := app.ResolveMode(id)
	if err != nil {
		return err
	}
	m.mode = mode.ID
	m.status = "Ready"
	m.resize()
	return nil
}

func (m *model) openModePicker() {
	items := make([]list.Item, 0, len(app.AvailableModes()))
	for _, mode := range app.AvailableModes() {
		items = append(items, modeChoice{mode: mode})
	}
	m.openSelector("Agent mode", items, m.modelDelegate())
	m.selectionKind = "agent-mode"
}

func (m *model) openSessionPicker(tree bool) tea.Cmd {
	browser, ok := m.service.(SessionBrowser)
	if !ok {
		m.status = "Session navigation is unavailable in this chat"
		return nil
	}
	m.actionPending = true
	m.status = "Loading saved sessions…"
	return func() tea.Msg {
		values, err := browser.SessionChoices(m.ctx)
		return sessionsMsg{values: values, err: err, tree: tree}
	}
}

func (m *model) openSettingsPicker() tea.Cmd {
	manager, ok := m.service.(SettingsManager)
	if !ok {
		m.status = "Settings controls are unavailable in this chat"
		return nil
	}
	m.actionPending = true
	m.status = "Loading workspace settings…"
	return func() tea.Msg { return settingsMsg{values: manager.Settings()} }
}

func (m *model) openMessagePicker() {
	items := make([]list.Item, 0)
	for index, entry := range m.conversationEntries {
		if entry.kind == conversationUser {
			items = append(items, messageChoice{index: index, entry: entry})
		}
	}
	if len(items) == 0 {
		m.status = "No user messages are available in this conversation"
		return
	}
	m.openSelector("Find a user message", items, m.modelDelegate())
	m.selectionKind = "user-messages"
}

func (m *model) loadSessionView(view SessionView) {
	m.selectedSession = &view.Summary
	m.approval = nil
	m.lastError = ""
	m.runDetails = fmt.Sprintf("Session: %s\nRun: %s\nState: %s\nCheckpoint: %d", view.Summary.SessionID, view.Summary.ID, view.Summary.State, view.Summary.Checkpoint)
	m.newConversationItems = false
	m.conversationEntries = nil
	m.entryIndexes = make(map[string]int)
	m.queuedMessages = nil
	m.text = ""
	m.streamBuffer = nil
	m.currentAssistantID = ""
	for _, message := range view.Messages {
		kind, state := conversationAssistant, "Opera"
		if message.Role == "You" {
			kind, state = conversationUser, "You"
		}
		m.appendEntry(conversationEntry{id: message.ID, kind: kind, state: state, text: message.Text})
	}
	if len(view.Messages) == 0 {
		m.appendEntry(conversationEntry{id: "history-empty:" + view.Summary.ID, kind: conversationSystem, text: "No readable user or assistant messages are stored at this checkpoint."})
	}
	m.resize()
	m.conversation.GotoBottom()
}

func (m *model) loadSession(id string, version uint64, mode string) tea.Cmd {
	browser, ok := m.service.(SessionBrowser)
	if !ok {
		m.status = "Session navigation is unavailable in this chat"
		return nil
	}
	m.selecting = false
	m.actionPending = true
	m.status = "Loading saved conversation…"
	return func() tea.Msg {
		view, err := browser.ReadSession(m.ctx, id, version)
		return sessionViewMsg{value: view, err: err, mode: mode}
	}
}

func (m *model) clearConversation() {
	m.previous = ""
	m.resume = app.Request{}
	m.retryPrevious = false
	m.stream = nil
	m.approval = nil
	m.lastError = ""
	m.runDetails = ""
	m.text = ""
	m.streamBuffer = nil
	m.conversationEntries = nil
	m.entryIndexes = make(map[string]int)
	m.queuedMessages = nil
	m.currentAssistantID = ""
	m.lastSequence = 0
	m.followUp = false
	m.todoTasks = nil
	m.todoRunID = ""
	m.todoGeneration++
	m.subagents = nil
	m.subagentRunID = ""
	m.subagentGeneration++
	m.selectedSession = nil
	m.panelOpen = false
	m.panelTitle = ""
	m.newConversationItems = false
	m.editor.Reset()
	m.refreshContext()
	m.resize()
}

func (m *model) openSessionActions(summary SessionSummary) {
	m.selectedSession = &summary
	items := []list.Item{
		sessionActionChoice{key: "view", title: "View conversation history", description: "Read saved messages without resuming the run"},
	}
	if summary.State == "completed" {
		items = append(items, sessionActionChoice{key: "continue", title: "Continue this session", description: "The next message starts from the selected completed checkpoint"})
		items = append(items, sessionActionChoice{key: "branch-latest", title: fmt.Sprintf("Create branch from checkpoint %d", summary.Checkpoint), description: "Create a new durable session; the original history stays unchanged"})
	} else if summary.State != "in_doubt" {
		items = append(items, sessionActionChoice{key: "resume", title: "Resume saved checkpoint", description: "Resume run " + summary.ID + " using the existing recovery checks"})
	} else {
		items = append(items, sessionActionChoice{key: "resume", title: "Inspect recovery state", description: "Resume is blocked until uncertain operations are reconciled"})
	}
	if len(summary.Checkpoints) > 0 {
		items = append(items, sessionActionChoice{key: "checkpoints", title: "Browse checkpoints", description: "Select a saved checkpoint to inspect or branch from"})
	}
	m.openSelector("Session · "+summary.Title, items, m.modelDelegate())
	m.selectionKind = "session-actions"
}

func (m *model) openCheckpointPicker(summary SessionSummary) {
	items := make([]list.Item, 0, len(summary.Checkpoints))
	for _, version := range summary.Checkpoints {
		items = append(items, checkpointChoice{SessionSummary: summary, Version: version})
	}
	if len(items) == 0 {
		m.status = "No saved checkpoints are available for this run"
		return
	}
	m.openSelector("Checkpoints · "+summary.SessionID, items, m.modelDelegate())
	m.selectionKind = "checkpoint-list"
}

func (m *model) openBranchConfirmation(summary SessionSummary, version uint64) tea.Cmd {
	m.selectedSession = &summary
	m.selectedCheckpoint = version
	form := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Key("confirm").Title(fmt.Sprintf("Create a new branch from run %s at checkpoint %d?", summary.ID, version)).Description("This writes a new session record. The source session and its effects are not changed.").Affirmative("Create branch").Negative("Cancel"),
	)).WithWidth(max(1, m.contentWidth()-4)).WithHeight(7).WithShowHelp(true).WithTheme(approvalTheme(m.dark))
	return m.openForm("Create session branch", form, "branch-confirm", "", "")
}

func (m *model) openForm(title string, form *huh.Form, kind, keyName, value string) tea.Cmd {
	m.panelTitle = title
	m.panel = viewport.New(viewport.WithWidth(max(1, m.contentWidth()-4)), viewport.WithHeight(m.viewerHeight()))
	m.panel.SetContent("")
	m.formHeight = m.viewerHeight()
	switch kind {
	case "thinking":
		m.formHeight = min(m.formHeight, 10)
	case "policy-select":
		m.formHeight = min(m.formHeight, 8)
	case "context-add":
		m.formHeight = min(m.formHeight, 6)
	case "branch-confirm", "policy-confirm":
		m.formHeight = min(m.formHeight, 7)
	}
	m.approvalForm = form.WithWidth(max(1, m.contentWidth()-4)).WithHeight(m.formHeight).WithTheme(approvalTheme(m.dark))
	m.formKind, m.formKey, m.formValue = kind, keyName, value
	m.panelOpen = true
	m.selecting = false
	m.editor.Blur()
	m.resize()
	return m.approvalForm.Init()
}

func (m *model) openSetting(setting SettingItem) tea.Cmd {
	if strings.HasPrefix(setting.Key, "context.remove:") {
		return m.applySetting("context.remove", strings.TrimPrefix(setting.Key, "context.remove:"))
	}
	switch setting.Key {
	case "provider", "model":
		return m.openModelPicker()
	case "thinking":
		options := []huh.Option[string]{
			huh.NewOption("Off", "off"), huh.NewOption("Minimal", "minimal"), huh.NewOption("Low", "low"),
			huh.NewOption("Medium", "medium"), huh.NewOption("High", "high"), huh.NewOption("Extra high", "xhigh"), huh.NewOption("Maximum", "max"),
		}
		form := huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Key("value").Title("Reasoning level").Description("Some models do not support every level.").Options(options...).Value(&setting.Value).Height(8))).WithWidth(max(1, m.contentWidth()-4)).WithHeight(10).WithShowHelp(true)
		return m.openForm("Thinking settings", form, "thinking", setting.Key, setting.Value)
	case "ui.theme":
		options := []themeChoice{
			{value: "system", name: "System", description: "Follow the terminal background", current: setting.Value == "system"},
			{value: "midnight", name: "Midnight", description: "Violet accents on deep gray", current: setting.Value == "midnight"},
			{value: "ocean", name: "Ocean", description: "Blue and cyan accents", current: setting.Value == "ocean"},
			{value: "forest", name: "Forest", description: "Green and moss accents", current: setting.Value == "forest"},
			{value: "sunset", name: "Sunset", description: "Warm amber and coral accents", current: setting.Value == "sunset"},
			{value: "mono", name: "Mono", description: "Grayscale palette", current: setting.Value == "mono"},
			{value: "dark", name: "Dark", description: "Classic cool blue palette", current: setting.Value == "dark"},
			{value: "light", name: "Light", description: "Classic light-terminal palette", current: setting.Value == "light"},
		}
		items := make([]list.Item, 0, len(options))
		for _, option := range options {
			items = append(items, option)
		}
		m.openSelector("Interface theme", items, m.modelDelegate())
		m.selectionKind = "theme"
		return nil
	case "policy":
		options := []huh.Option[string]{
			huh.NewOption("Read only · block workspace writes", "read-only"),
			huh.NewOption("Ask · request approval for effects", "ask"),
			huh.NewOption("Trusted · allow effects without per-operation approval", "trusted"),
		}
		form := huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Key("value").Title("Workspace tool policy").Description("Changing this affects which tool effects can run.").Options(options...).Value(&setting.Value).Height(5))).WithWidth(max(1, m.contentWidth()-4)).WithHeight(8).WithShowHelp(true)
		return m.openForm("Policy settings", form, "policy-select", setting.Key, setting.Value)
	case "context":
		return m.openContextInput()
	default:
		return nil
	}
}

func (m *model) openModelPicker() tea.Cmd {
	providers, ok := m.service.(ProviderModelChooser)
	if ok {
		m.actionPending = true
		m.status = "Loading available models…"
		m.editor.Blur()
		return func() tea.Msg {
			type catalogResult struct {
				index  int
				values []ModelOption
				err    error
			}
			configured := providers.ModelProviders()
			results := make(chan catalogResult, len(configured))
			for index, provider := range configured {
				go func(index int, id string) {
					values, err := providers.ModelCatalog(m.ctx, id)
					results <- catalogResult{index: index, values: values, err: err}
				}(index, provider.ID)
			}
			catalogs := make([]catalogResult, len(configured))
			var firstErr error
			for range configured {
				result := <-results
				catalogs[result.index] = result
			}
			var values []ModelOption
			for _, result := range catalogs {
				if result.err != nil {
					if firstErr == nil {
						firstErr = result.err
					}
					continue
				}
				values = append(values, result.values...)
			}
			if len(values) == 0 {
				return modelCatalogMsg{err: firstErr}
			}
			return modelCatalogMsg{values: values}
		}
	}
	chooser, ok := m.service.(ModelChooser)
	if !ok {
		m.status = "Model selection is unavailable in this chat"
		return nil
	}
	m.actionPending = true
	m.status = "Loading the current provider's model catalog…"
	return func() tea.Msg { values, err := chooser.ModelChoices(m.ctx); return choicesMsg{values, err} }
}

func (m *model) openPolicyConfirmation(value string) tea.Cmd {
	form := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Key("confirm").Title("Apply workspace policy: " + value + "?").Description("The new policy controls tool effects in future executions.").Affirmative("Apply policy").Negative("Cancel"),
	)).WithWidth(max(1, m.contentWidth()-4)).WithHeight(7).WithShowHelp(true)
	return m.openForm("Confirm policy change", form, "policy-confirm", "policy", value)
}

func (m *model) openContextInput() tea.Cmd {
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Key("path").Title("Workspace file to attach").Description("Use a path relative to the workspace. The file is validated before use."),
	)).WithWidth(max(1, m.contentWidth()-4)).WithHeight(6).WithShowHelp(true)
	return m.openForm("Attach context", form, "context-add", "context.add", "")
}

func (m model) finishForm() (tea.Model, tea.Cmd) {
	form, kind, keyName, value := m.approvalForm, m.formKind, m.formKey, m.formValue
	m.approvalForm = nil
	m.panelOpen = false
	m.formKind, m.formKey, m.formValue = "", "", ""
	m.formHeight = 0
	m.resize()
	switch kind {
	case "approval":
		decision := form.GetString("decision")
		if decision == "allow" || decision == "deny" {
			return m, m.resolveApproval(decision == "allow")
		}
	case "thinking":
		return m, m.applySetting(keyName, form.GetString("value"))
	case "policy-select":
		selected := form.GetString("value")
		if selected == value {
			m.status = "Policy is unchanged"
			return m, m.editor.Focus()
		}
		return m, m.openPolicyConfirmation(selected)
	case "policy-confirm":
		if form.GetBool("confirm") {
			return m, m.applySetting(keyName, value)
		}
	case "context-add":
		return m, m.applySetting("context.add", form.GetString("path"))
	case "branch-confirm":
		if form.GetBool("confirm") && m.selectedSession != nil {
			return m, m.branch(*m.selectedSession, m.selectedCheckpoint)
		}
	}
	return m, m.editor.Focus()
}

func (m *model) applySetting(keyName, value string) tea.Cmd {
	manager, ok := m.service.(SettingsManager)
	if !ok {
		m.status = "Settings controls are unavailable in this chat"
		return nil
	}
	m.actionPending = true
	m.status = "Applying " + keyName + " setting…"
	return func() tea.Msg {
		message, err := manager.SetSetting(m.ctx, keyName, value)
		return settingResultMsg{key: keyName, value: value, message: message, err: err}
	}
}

func (m *model) branch(summary SessionSummary, version uint64) tea.Cmd {
	browser, ok := m.service.(SessionBrowser)
	if !ok {
		m.status = "Session branching is unavailable in this chat"
		return nil
	}
	m.actionPending = true
	m.status = fmt.Sprintf("Creating a branch from checkpoint %d…", version)
	return func() tea.Msg {
		id, err := browser.BranchSession(m.ctx, summary.ID, version)
		return branchMsg{id: id, version: version, err: err}
	}
}

func (m model) selectCurrent() (tea.Model, tea.Cmd) {
	item := m.selector.SelectedItem()
	kind := m.selectionKind
	m.selecting = false
	m.modelMenu = false
	m.selectionKind = ""
	m.resize()
	switch selected := item.(type) {
	case loginChoice:
		return m, m.beginLogin(selected)
	case choice:
		m.editor.Focus()
		if commander, ok := m.service.(Commander); ok {
			m.actionPending = true
			return m, func() tea.Msg {
				value, err := commander.Command(m.ctx, "/model "+string(selected))
				return panelMsg{text: value, err: err, title: "Model"}
			}
		}
	case modeChoice:
		if kind == "agent-mode" {
			m.editor.Focus()
			if err := m.selectMode(selected.mode.ID); err != nil {
				m.status = err.Error()
			}
			return m, nil
		}
	case providerChoice:
		if providers, ok := m.service.(ProviderModelChooser); ok {
			m.selectedProvider = selected.ID
			m.actionPending = true
			m.editor.Blur()
			return m, func() tea.Msg {
				values, err := providers.ModelCatalog(m.ctx, selected.ID)
				return modelCatalogMsg{provider: selected.ID, values: values, err: err}
			}
		}
	case modelChoice:
		if commander, ok := m.service.(Commander); ok {
			m.actionPending = true
			m.editor.Blur()
			command := "/model " + selected.Provider + " " + selected.ID
			return m, func() tea.Msg {
				_, err := commander.Command(m.ctx, command)
				return modelSelectionMsg{err: err}
			}
		}
	case sessionChoice:
		if kind == "session-tree" {
			m.openSessionActions(selected.SessionSummary)
			return m, nil
		}
		return m, m.loadSession(selected.ID, 0, "resume")
	case sessionActionChoice:
		if kind == "session-actions" && m.selectedSession != nil {
			summary := *m.selectedSession
			switch selected.key {
			case "view":
				return m, m.loadSession(summary.ID, 0, "view")
			case "continue", "resume":
				return m, m.loadSession(summary.ID, 0, "resume")
			case "branch-latest":
				return m, m.openBranchConfirmation(summary, summary.Checkpoint)
			case "checkpoints":
				m.openCheckpointPicker(summary)
				return m, nil
			}
		}
		if kind == "checkpoint-actions" && m.selectedSession != nil {
			summary := *m.selectedSession
			switch selected.key {
			case "checkpoint-view":
				return m, m.loadSession(summary.ID, m.selectedCheckpoint, "view")
			case "checkpoint-branch":
				return m, m.openBranchConfirmation(summary, m.selectedCheckpoint)
			}
		}
		if kind == "context-actions" {
			if selected.key == "context-add" {
				return m, m.openContextInput()
			}
			if strings.HasPrefix(selected.key, "context-remove:") {
				return m, m.applySetting("context.remove", strings.TrimPrefix(selected.key, "context-remove:"))
			}
		}
	case checkpointChoice:
		m.selectedSession = &selected.SessionSummary
		m.selectedCheckpoint = selected.Version
		items := []list.Item{
			sessionActionChoice{key: "checkpoint-view", title: "View this checkpoint", description: fmt.Sprintf("Read run %s at checkpoint %d without resuming", selected.ID, selected.Version)},
			sessionActionChoice{key: "checkpoint-branch", title: "Create branch from this checkpoint", description: fmt.Sprintf("Create a new session based on run %s at checkpoint %d", selected.ID, selected.Version)},
		}
		m.openSelector(fmt.Sprintf("Checkpoint %d", selected.Version), items, m.modelDelegate())
		m.selectionKind = "checkpoint-actions"
		return m, nil
	case messageChoice:
		m.jumpToEntry(selected.index)
		return m, m.editor.Focus()
	case settingChoice:
		if strings.HasPrefix(selected.Key, "context.remove:") {
			return m, m.applySetting("context.remove", strings.TrimPrefix(selected.Key, "context.remove:"))
		}
		return m, m.openSetting(selected.SettingItem)
	case themeChoice:
		return m, m.applySetting("ui.theme", selected.value)
	}
	m.editor.Focus()
	return m, nil
}

func (m *model) jumpToEntry(index int) {
	if index < 0 || index >= len(m.conversationEntries) {
		return
	}
	offset := 0
	width := max(1, m.conversationWidth())
	for i := 0; i < index; i++ {
		wrapped := ansi.Hardwrap(m.renderEntry(m.conversationEntries[i]), width, true)
		offset += strings.Count(wrapped, "\n") + 1
	}
	m.conversation.SetYOffset(offset)
	m.newConversationItems = !m.conversation.AtBottom()
}

func approvalText(view *app.Approval) string {
	var args any
	arguments := string(view.Arguments)
	if json.Unmarshal(view.Arguments, &args) == nil {
		if pretty, err := json.MarshalIndent(args, "", "  "); err == nil {
			arguments = string(pretty)
		}
	}
	return fmt.Sprintf("Tool: %s\nPath: %s\n\nArguments\n%s\n\nChanges\n%s\n\nAgent: %s\nTask: %s\nApproval: %s\n\nEsc back · /approve allow · /deny reject", view.Tool, view.Path, arguments, view.Diff, view.Agent, view.TaskID, view.ID)
}

func (m *model) openApproval(view *app.Approval) tea.Cmd {
	m.panelTitle = "Review operation"
	m.panel = viewport.New(viewport.WithWidth(max(1, m.contentWidth()-4)), viewport.WithHeight(m.viewerHeight()))
	m.panel.SoftWrap = true
	m.panel.SetContent(approvalPanelContent(view, m.contentWidth()-4, m.theme()))
	m.panelOpen = true
	m.editor.Blur()
	decision := huh.NewSelect[string]().
		Key("decision").
		Title("Operation decision").
		Description("Choose whether to authorize this exact pending operation").
		Options(huh.NewOption("Allow operation", "allow"), huh.NewOption("Deny operation", "deny")).
		Height(3)
	m.approvalForm = huh.NewForm(huh.NewGroup(decision)).WithWidth(max(1, m.contentWidth()-4)).WithHeight(5).WithShowHelp(true).WithTheme(approvalTheme(m.dark))
	m.formKind = "approval"
	return m.approvalForm.Init()
}

func approvalTheme(dark bool) huh.Theme {
	return huh.ThemeFunc(func(bool) *huh.Styles { return huh.ThemeBase16(dark) })
}

func (m *model) resolveApproval(allow bool) tea.Cmd {
	if m.approval == nil {
		return nil
	}
	request := m.resume
	if allow {
		request.Approve = m.approval.ID
	} else {
		request.Deny = m.approval.ID
	}
	m.approval = nil
	m.approvalForm = nil
	m.panelOpen = false
	m.formKind = ""
	m.actionPending = true
	if allow {
		m.status = "Approval accepted · resuming operation"
	} else {
		m.status = "Approval denied · recording decision"
	}
	return tea.Batch(m.editor.Focus(), m.start(request))
}

func approvalPanelContent(view *app.Approval, width int, t palette) string {
	arguments := string(view.Arguments)
	var value any
	if json.Unmarshal(view.Arguments, &value) == nil {
		if formatted, err := json.MarshalIndent(value, "", "  "); err == nil {
			arguments = string(formatted)
		}
	}
	lines := []string{
		"Tool: " + safeText(view.Tool),
		"Path: " + safeText(view.Path),
		"Agent: " + safeText(view.Agent),
		"Task: " + safeText(view.TaskID),
		"Approval ID: " + safeText(view.ID),
		"",
		"Arguments",
		safeText(arguments),
		"",
		"Changes",
		approvalDiff(view.Diff, max(1, width-2), t),
	}
	return strings.Join(lines, "\n")
}

func (m model) chatRequest(text string) app.Request {
	request := app.Request{Prompt: text, ContinueFrom: m.previous, Mode: m.mode}
	if m.retryPrevious && m.previous != "" {
		request.ID = m.previous
		request.ContinueFrom = ""
	}
	return request
}

func parseImageCommand(value string) (path, prompt string, ok bool) {
	value = strings.TrimSpace(value)
	path, prompt, ok = strings.Cut(value, " ")
	path, prompt = strings.TrimSpace(path), strings.TrimSpace(prompt)
	return path, prompt, ok && path != "" && prompt != ""
}
