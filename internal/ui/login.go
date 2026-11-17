package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/diasYuri/opera/internal/app"
)

type loginChoice struct{ app.LoginProvider }

func (c loginChoice) Title() string       { return safeText(c.Name) }
func (c loginChoice) Description() string { return safeText(c.LoginProvider.Description) }
func (c loginChoice) FilterValue() string { return c.Name + " " + c.LoginProvider.Description }

type loginFlow struct {
	ctx    context.Context
	cancel context.CancelFunc
	events chan loginEvent
	name   string
}
type loginEvent struct {
	flow   *loginFlow
	notice app.LoginNotice
	err    error
	done   bool
}

func (m *model) openLoginProviders() {
	auth, ok := m.service.(app.Authenticator)
	if !ok {
		m.status = "Login is unavailable in this session"
		return
	}
	providers := auth.LoginProviders()
	if len(providers) == 0 {
		m.status = "No login providers are available"
		return
	}
	items := make([]list.Item, 0, len(providers))
	for _, provider := range providers {
		items = append(items, loginChoice{provider})
	}
	delegate := m.modelDelegate()
	delegate.ShowDescription = true
	delegate.SetHeight(2)
	delegate.Styles.NormalDesc = m.theme().muted.PaddingLeft(2)
	delegate.Styles.SelectedDesc = m.theme().muted.PaddingLeft(2)
	delegate.Styles.DimmedDesc = m.theme().muted.PaddingLeft(2)
	m.openSelector("Connect a provider", items, delegate)
	m.overlay.selectionKind = "login"
	m.lastError = ""
}

func (m *model) openSelector(title string, items []list.Item, delegate list.DefaultDelegate) {
	m.overlay.modelMenu = false
	m.overlay.selector = list.New(items, delegate, max(1, m.contentWidth()-4), m.viewerHeight())
	m.overlay.selector.Title = title
	m.overlay.selector.Styles = list.DefaultStyles(m.dark)
	m.overlay.selector.Styles.Title = m.theme().accent.Bold(true)
	m.overlay.selector.SetShowTitle(false)
	m.overlay.selector.SetShowStatusBar(false)
	m.overlay.selector.SetShowHelp(false)
	m.overlay.showSelector(m.overlay.selector)
	m.composer.editor.Blur()
	m.resize()
}

func (m *model) beginLogin(provider loginChoice) tea.Cmd {
	auth, ok := m.service.(app.Authenticator)
	if !ok {
		return nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	flow := &loginFlow{ctx: ctx, cancel: cancel, events: make(chan loginEvent, 1), name: provider.Name}
	m.login = flow
	m.overlay.selecting = false
	m.actionPending = true
	m.composer.editor.Blur()
	m.status = "Connecting to " + provider.Name + "…"
	m.activity.Start()
	work := func() tea.Msg {
		defer close(flow.events)
		err := auth.Login(ctx, provider.ID, func(notice app.LoginNotice) error {
			select {
			case flow.events <- loginEvent{flow: flow, notice: notice}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		// Always acknowledge completion/cancellation before enabling another action.
		select {
		case flow.events <- loginEvent{flow: flow, done: true, err: err}:
		case <-m.ctx.Done():
		}
		return nil
	}
	return tea.Batch(work, awaitLogin(flow), m.animate())
}

func awaitLogin(flow *loginFlow) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-flow.events
		if !ok {
			return nil
		}
		return event
	}
}

func (m model) updateLogin(event loginEvent) (tea.Model, tea.Cmd) {
	if m.login != event.flow {
		return m, nil
	}
	if !event.done {
		if m.login.ctx != nil && m.login.ctx.Err() != nil {
			return m, awaitLogin(m.login)
		}
		if event.notice.Prompt != nil {
			m.loginPrompt = event.notice.Prompt
			if m.loginPrompt.Kind == "secret" {
				input := huh.NewInput().Key("value").Title(m.loginPrompt.Message).Placeholder(m.loginPrompt.Placeholder).EchoMode(huh.EchoModePassword).Validate(func(value string) error {
					if strings.TrimSpace(value) == "" {
						return errors.New("a value is required")
					}
					return nil
				})
				form := huh.NewForm(huh.NewGroup(input)).WithShowHelp(true)
				cmd := m.openForm(m.loginPrompt.Title, form, "login-secret", "", "")
				m.status = event.notice.Message
				return m, tea.Batch(awaitLogin(event.flow), cmd)
			}
			items := make([]list.Item, 0, len(m.loginPrompt.Options))
			for _, option := range m.loginPrompt.Options {
				items = append(items, loginStorageChoice{option})
			}
			delegate := m.modelDelegate()
			delegate.ShowDescription = true
			delegate.SetHeight(2)
			delegate.Styles.NormalDesc = m.theme().muted.PaddingLeft(2)
			delegate.Styles.SelectedDesc = m.theme().muted.PaddingLeft(2)
			m.openSelector(m.loginPrompt.Title, items, delegate)
			m.overlay.selectionKind = "login-storage"
			m.status = event.notice.Message
			return m, awaitLogin(event.flow)
		}
		m.status = event.notice.Message
		if event.notice.URL != "" {
			m.status += "\n" + safeText(event.notice.URL)
			return m, awaitLogin(event.flow)
		}
		return m, awaitLogin(event.flow)
	}
	m.login.cancel()
	m.login = nil
	m.loginPrompt = nil
	m.overlay.selecting = false
	m.actionPending = false
	if event.err != nil {
		if errors.Is(event.err, context.Canceled) {
			m.status = "Login cancelled"
			return m, m.composer.editor.Focus()
		}
		m.status = "Login failed · /login to try again"
		m.lastError = event.err.Error()
		m.appendEntry(conversationEntry{id: m.conversation.nextIdentity("login-error"), kind: conversationError, text: event.err.Error()})
		return m, m.composer.editor.Focus()
	}
	m.refreshContext()
	m.code = 0
	m.status = fmt.Sprintf("Connected to %s · choose a model", event.flow.name)
	if chooser, ok := m.service.(ModelChooser); ok {
		m.actionPending = true
		return m, func() tea.Msg { values, err := chooser.ModelChoices(m.ctx); return choicesMsg{values, err} }
	}
	return m, m.composer.editor.Focus()
}

type loginStorageChoice struct{ app.LoginOption }

func (c loginStorageChoice) Title() string       { return safeText(c.Name) }
func (c loginStorageChoice) Description() string { return safeText(c.LoginOption.Description) }
func (c loginStorageChoice) FilterValue() string { return c.Name + " " + c.LoginOption.Description }

func (m model) updateLoginPrompt(message tea.Msg, key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.loginPrompt != nil && m.loginPrompt.Kind == "secret" {
		if key.String() == "esc" || key.String() == "ctrl+c" {
			if m.login != nil {
				m.login.cancel()
			}
			m.approvalForm = nil
			m.loginPrompt = nil
			m.formKind = ""
			m.formHeight = 0
			m.overlay.dismissPanel()
			m.resize()
			return m, m.composer.editor.Focus()
		}
		if m.approvalForm == nil {
			return m, nil
		}
		form := m.approvalForm
		_, cmd := form.Update(message)
		if form.State == huh.StateCompleted {
			return m.finishForm()
		}
		if form.State == huh.StateAborted {
			if m.login != nil {
				m.login.cancel()
			}
			m.approvalForm = nil
			m.loginPrompt = nil
			m.formKind = ""
			m.formHeight = 0
			m.overlay.dismissPanel()
			m.resize()
			return m, m.composer.editor.Focus()
		}
		return m, cmd
	}
	switch key.String() {
	case "esc", "ctrl+c":
		m.login.cancel()
		m.loginPrompt = nil
		m.overlay.selecting = false
		m.status = "Cancelling login…"
		return m, nil
	case "enter":
		selected, ok := m.overlay.selector.SelectedItem().(loginStorageChoice)
		if !ok {
			return m, nil
		}
		prompt, ctx := m.loginPrompt, m.login.ctx
		m.loginPrompt = nil
		m.overlay.selecting = false
		if selected.ID == "file" {
			m.status = "Saving credentials to private local file…"
		} else {
			m.status = "Retrying credential storage…"
		}
		return m, func() tea.Msg {
			select {
			case prompt.Response <- selected.ID:
			case <-ctx.Done():
			}
			return nil
		}
	}
	var command tea.Cmd
	m.overlay.selector, command = m.overlay.selector.Update(message)
	return m, command
}
