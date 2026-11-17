package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/diasYuri/opera/internal/modules/execution"
)

type loginExecutor struct {
	terminalExecutor
	providers     []execution.LoginProvider
	selected      string
	waitForCancel bool
	loginError    error
}

func (e *loginExecutor) LoginProviders() []execution.LoginProvider { return e.providers }
func (e *loginExecutor) Login(ctx context.Context, id string, notify func(execution.LoginNotice) error) error {
	e.selected = id
	if err := notify(execution.LoginNotice{URL: "https://example.test/authorize?state=fixture", Message: "Finish sign-in in your browser"}); err != nil {
		return err
	}
	if e.waitForCancel {
		<-ctx.Done()
		return ctx.Err()
	}
	return e.loginError
}
func (e *loginExecutor) ModelChoices(context.Context) ([]string, error) {
	return []string{"openai-subscription account-model"}, nil
}

func loginModel(t *testing.T, e *loginExecutor) model {
	t.Helper()
	m := newModel(context.Background(), e, execution.Request{}, true)
	m.composer.editor.SetValue("/login")
	m.refreshCompletions()
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return next.(model)
}

func TestLoginSubmenuUsesAvailableProvidersAndKeyboard(t *testing.T) {
	e := &loginExecutor{providers: []execution.LoginProvider{{ID: "openai", Name: "OpenAI", Description: "ChatGPT subscription"}, {ID: "fixture", Name: "Fixture provider", Description: "Future provider"}}}
	m := loginModel(t, e)
	if !m.overlay.selecting || m.overlay.selectionKind != "login" || len(m.overlay.selector.Items()) != 2 || !strings.Contains(m.View().Content, "Connect a provider") {
		t.Fatal("missing provider submenu")
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(model)
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	if m.login == nil || m.overlay.selecting || !m.actionPending || cmd == nil {
		t.Fatal("provider selection did not start login")
	}
	batch := cmd().(tea.BatchMsg)
	done := make(chan struct{})
	go func() { _ = batch[0](); close(done) }()
	notice := awaitLogin(m.login)().(loginEvent)
	next, _ = m.Update(notice)
	m = next.(model)
	if !strings.Contains(m.View().Content, "Finish sign-in") || strings.Contains(m.View().Content, "Starting…") {
		t.Fatal("login progress not visible")
	}
	completion := awaitLogin(m.login)().(loginEvent)
	<-done
	if e.selected != "fixture" {
		t.Fatal("submenu did not use selected registry entry")
	}
	next, cmd = m.Update(completion)
	m = next.(model)
	if m.login != nil || cmd == nil {
		t.Fatal("successful login did not request models")
	}
	next, _ = m.Update(cmd())
	m = next.(model)
	if !m.overlay.selecting || m.overlay.selectionKind != "model" || !strings.Contains(m.View().Content, "account-model") {
		t.Fatal("account models not shown after login")
	}
}

func TestLoginCancellationKeepsSessionAndRestoresComposer(t *testing.T) {
	e := &loginExecutor{providers: []execution.LoginProvider{{ID: "openai", Name: "OpenAI"}}, waitForCancel: true}
	m := loginModel(t, e)
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	flow := m.login
	batch := cmd().(tea.BatchMsg)
	done := make(chan struct{})
	go func() { _ = batch[0](); close(done) }()
	next, _ = m.Update(awaitLogin(flow)())
	m = next.(model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(model)
	if m.login == nil || !m.actionPending {
		t.Fatal("login re-enabled actions before cancellation completed")
	}
	next, _ = m.Update(awaitLogin(flow)())
	m = next.(model)
	<-done
	if m.login != nil || m.actionPending || !m.composer.editor.Focused() || m.status != "Login cancelled" {
		t.Fatal("cancellation did not restore session", m.status)
	}
	next, _ = m.Update(loginEvent{flow: flow, done: true})
	m = next.(model)
	if m.overlay.selecting || m.login != nil {
		t.Fatal("stale login event changed the session")
	}
}

func TestLoginErrorsAndBusyStates(t *testing.T) {
	for _, busy := range []string{"execution", "approval"} {
		t.Run(busy, func(t *testing.T) {
			m := newModel(context.Background(), &loginExecutor{}, execution.Request{}, true)
			if busy == "execution" {
				m.stream = &execution.Execution{}
			} else {
				m.approval = &execution.Approval{ID: "approval"}
			}
			m.composer.editor.SetValue("/login")
			next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			m = next.(model)
			if m.overlay.selecting || m.login != nil || !strings.Contains(m.status, "Resolve") {
				t.Fatal("busy session accepted login")
			}
		})
	}
	m := newModel(context.Background(), &loginExecutor{}, execution.Request{}, true)
	m.login = &loginFlow{cancel: func() {}, name: "OpenAI"}
	next, _ := m.Update(loginEvent{flow: m.login, done: true, err: errors.New("fixture authorization failed")})
	m = next.(model)
	if m.lastError == "" || m.login != nil || !m.composer.editor.Focused() {
		t.Fatal("login error left session blocked")
	}
}

func TestLoginStoragePromptContinuesTheSameFlow(t *testing.T) {
	m := newModel(context.Background(), &loginExecutor{}, execution.Request{}, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	flow := &loginFlow{ctx: ctx, cancel: cancel, name: "OpenAI", events: make(chan loginEvent, 1)}
	m.login = flow
	m.actionPending = true
	response := make(chan string, 1)
	prompt := &execution.LoginPrompt{Title: "Credential storage", Options: []execution.LoginOption{{ID: "retry", Name: "Retry system keyring"}, {ID: "file", Name: "Use private local file", Description: "Unencrypted"}}, Response: response}
	next, _ := m.Update(loginEvent{flow: flow, notice: execution.LoginNotice{Prompt: prompt}})
	m = next.(model)
	if !m.overlay.selecting || !strings.Contains(m.View().Content, "Credential storage") || !strings.Contains(m.View().Content, "Unencrypted") {
		t.Fatal("storage recovery menu not visible")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(model)
	next, command := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	if command == nil || m.login != flow || m.overlay.selecting || m.loginPrompt != nil {
		t.Fatal("storage choice restarted or ended OAuth")
	}
	_ = command()
	if choice := <-response; choice != "file" {
		t.Fatal("wrong storage choice", choice)
	}
	next, command = m.Update(loginEvent{flow: flow, done: true})
	m = next.(model)
	if m.login != nil || command == nil {
		t.Fatal("recovered login did not continue to model selection")
	}
}
