package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/diasYuri/opera/internal/adapters/context/filesystem"
	"github.com/diasYuri/opera/internal/core"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/execution"
	"github.com/diasYuri/opera/internal/modules/providers/catalog"
	ui "github.com/diasYuri/opera/internal/ui/tui"
	"github.com/diasYuri/y/pkg/ai"
)

type loginMethod struct {
	execution.LoginProvider
	descriptor *providercatalog.LoginDescriptor
}

// chatSession can show commands and sign-in before an execution runtime exists.
// UI actions are serialized; auth notices never write directly to the terminal.
type chatSession struct {
	completionDiscovery composerDiscovery
	mu                  sync.Mutex
	app                 App
	cfg                 config.Resolved
	opts                core.Options
	runtime             *core.Runtime
	methods             []loginMethod
	openBrowser         func(context.Context, string) error
}

func newChatSession(a App, cfg config.Resolved, opts core.Options, r *core.Runtime) *chatSession {
	if opts.ProviderCatalog == nil {
		opts.ProviderCatalog = a.ProviderCatalog
	}
	values := make(map[string]config.Value, len(cfg.Values))
	for k, v := range cfg.Values {
		values[k] = v
	}
	cfg.Values = values
	if r != nil {
		_, provider, model := r.SessionInfo()
		cfg.Values["provider"] = config.Value{Value: provider, Source: "checkpoint"}
		cfg.Values["model"] = config.Value{Value: model, Source: "checkpoint"}
	}
	opts.ResumeID = ""
	s := &chatSession{app: a, cfg: cfg, opts: opts, runtime: r, openBrowser: openLoginBrowser}
	if catalog, err := s.providerCatalog(); err == nil {
		for _, descriptor := range catalog.Logins() {
			copy := descriptor
			copy.Methods = append([]providercatalog.LoginMethod(nil), descriptor.Methods...)
			s.methods = append(s.methods, loginMethod{
				LoginProvider: execution.LoginProvider{ID: descriptor.ID, Name: descriptor.Name, Description: descriptor.Description},
				descriptor:    &copy,
			})
		}
	}
	return s
}

func (s *chatSession) LoginProviders() []execution.LoginProvider {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]execution.LoginProvider, 0, len(s.methods))
	for _, method := range s.methods {
		out = append(out, method.LoginProvider)
	}
	return out
}

func (s *chatSession) Login(ctx context.Context, id string, notify func(execution.LoginNotice) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, method := range s.methods {
		if method.ID != id {
			continue
		}
		executionProvider := ""
		notifyFlow := func(notice execution.LoginNotice) error {
			if notice.URL == "" {
				return notify(notice)
			}
			u := notice.URL
			if err := notify(notice); err != nil {
				return err
			}
			if s.openBrowser != nil {
				browserCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				err := s.openBrowser(browserCtx, u)
				cancel()
				if err != nil {
					return notify(execution.LoginNotice{Message: "Open the authorization link above in your browser to continue."})
				}
			}
			return nil
		}
		var err error
		if method.descriptor != nil {
			var selected providercatalog.LoginMethod
			selected, err = chooseLoginMethod(ctx, notifyFlow, *method.descriptor)
			if err == nil {
				executionProvider = selected.ProviderID
				err = s.runLoginMethod(ctx, selected, notifyFlow)
			}
		}
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if s.runtime != nil {
			if err := s.runtime.Close(ctx); err != nil {
				return err
			}
			s.runtime = nil
		}
		s.cfg.Values["provider"] = config.Value{Value: executionProvider, Source: "interactive"}
		// A model from another provider must never be reused after sign-in.
		s.cfg.Values["model"] = config.Value{Value: "", Source: "interactive"}
		s.cfg.Values["base_url"] = config.Value{Value: "", Source: "interactive"}
		return nil
	}
	return errors.New("login provider is unavailable; use /login to see available providers")
}

func chooseLoginMethod(ctx context.Context, notify func(execution.LoginNotice) error, descriptor providercatalog.LoginDescriptor) (providercatalog.LoginMethod, error) {
	if len(descriptor.Methods) == 1 {
		return descriptor.Methods[0], nil
	}
	options := make([]execution.LoginOption, 0, len(descriptor.Methods))
	for _, method := range descriptor.Methods {
		options = append(options, execution.LoginOption{ID: method.ID, Name: method.Name, Description: method.Description})
	}
	response := make(chan string, 1)
	if err := notify(execution.LoginNotice{Message: "Choose a sign-in method.", Prompt: &execution.LoginPrompt{
		Kind: "select", Title: descriptor.Name + " sign-in", Options: options, Response: response,
	}}); err != nil {
		return providercatalog.LoginMethod{}, err
	}
	select {
	case id := <-response:
		for _, method := range descriptor.Methods {
			if method.ID == id {
				return method, nil
			}
		}
		return providercatalog.LoginMethod{}, errors.New("invalid sign-in method")
	case <-ctx.Done():
		return providercatalog.LoginMethod{}, ctx.Err()
	}
}

func (s *chatSession) runLoginMethod(ctx context.Context, method providercatalog.LoginMethod, notify func(execution.LoginNotice) error) error {
	if method.Kind == "api-key" {
		return s.apiKeyLogin(ctx, s.cfg, method.ProviderID, notify)
	}
	if method.Authenticator == nil {
		return errors.New("unsupported sign-in method")
	}
	return s.runProviderAuthentication(ctx, "login", method.Authenticator, notify)
}

func (s *chatSession) runProviderAuthentication(ctx context.Context, action string, handler providercatalog.Authenticator, notify func(execution.LoginNotice) error) error {
	store, err := s.openLoginStore(ctx, s.cfg, s.cfg.String("credentials.store"), s.cfg.Paths.Credentials, notify)
	if err != nil {
		return err
	}
	io := providercatalog.AuthIO{Store: store, Reference: s.cfg.String("credential_ref"), Accessible: s.cfg.Bool("ui.accessible"), Now: time.Now}
	io.Prompt = func(promptCtx context.Context, p providercatalog.LoginPrompt) (string, error) {
		options := make([]execution.LoginOption, len(p.Options))
		for i, v := range p.Options {
			options[i] = execution.LoginOption{ID: v.ID, Name: v.Name, Description: v.Description}
		}
		return promptLoginInput(promptCtx, notify, execution.LoginPrompt{Kind: p.Kind, Title: p.Title, Message: p.Message, Placeholder: p.Placeholder, Options: options})
	}
	io.Notify = func(noticeCtx context.Context, n providercatalog.LoginNotice) error {
		return notify(execution.LoginNotice{URL: n.URL, Message: n.Message})
	}
	io.Callback = func(callbackCtx context.Context, address, path, state string) (string, func(context.Context, time.Duration) (string, error), func(), error) {
		cb, err := startLoopbackOAuthCallback(callbackCtx, address, path, state)
		if err != nil {
			return "", nil, nil, err
		}
		return cb.RedirectURI, func(waitCtx context.Context, d time.Duration) (string, error) {
			value, err := cb.WaitBefore(waitCtx, d)
			if errors.Is(err, errOAuthCallbackTimeout) {
				return "", context.DeadlineExceeded
			}
			return value, err
		}, cb.Close, nil
	}
	_, err = handler.Authenticate(ctx, action, io)
	return err
}

func promptLoginInput(ctx context.Context, notify func(execution.LoginNotice) error, prompt execution.LoginPrompt) (string, error) {
	response := make(chan string, 1)
	prompt.Response = response
	if err := notify(execution.LoginNotice{Message: prompt.Message, Prompt: &prompt}); err != nil {
		return "", err
	}
	select {
	case value := <-response:
		return value, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (s *chatSession) SessionInfo() (string, string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg.Paths.Workspace, s.cfg.String("provider"), s.cfg.String("model")
}

func (s *chatSession) TaskSnapshot(ctx context.Context, runID string) (ui.TaskSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runtime == nil || s.runtime.Service == nil || s.runtime.Service.Work == nil {
		return ui.TaskSnapshot{}, nil
	}
	snapshot, err := s.runtime.Service.Work.Tasks(ctx, runID)
	if err != nil {
		return ui.TaskSnapshot{}, err
	}
	tasks := make([]ui.TaskItem, 0, len(snapshot.Tasks))
	for _, task := range snapshot.Tasks {
		tasks = append(tasks, ui.TaskItem{
			ID: task.ID, ParentID: task.ParentID, Name: task.Name,
			Description: task.Description, Order: task.Order, Status: string(task.Status),
		})
	}
	return ui.TaskSnapshot{Tasks: tasks}, nil
}

func (s *chatSession) SubagentSnapshot(ctx context.Context, runID string) ([]ui.SubagentItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runtime == nil || s.runtime.Service == nil || s.runtime.Service.Work == nil {
		return nil, nil
	}
	children, err := s.runtime.Service.Work.Children(ctx, runID)
	if err != nil {
		return nil, err
	}
	items := make([]ui.SubagentItem, 0, len(children))
	for _, child := range children {
		items = append(items, ui.SubagentItem{
			ID: child.ID, Name: child.Name, Status: string(child.Status), StartedAt: child.StartedAt,
		})
	}
	return items, nil
}

func (s *chatSession) ThemePreference() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg.String("ui.theme")
}

func (s *chatSession) UIContributions(ctx context.Context) ([]ui.ExtensionContribution, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	runtime := s.runtime
	s.mu.Unlock()
	if runtime == nil || runtime.Extensions == nil {
		return nil, nil
	}
	items := runtime.Extensions.VisualContributions()
	out := make([]ui.ExtensionContribution, 0, len(items))
	for _, item := range items {
		out = append(out, ui.ExtensionContribution{
			ID: item.ID, Kind: item.Kind, Target: item.Target, Text: item.Text,
			Detail: item.Detail, Trigger: item.Trigger, Value: item.Value,
		})
	}
	return out, nil
}

func (s *chatSession) AttachmentPreviews(ctx context.Context) ([]ui.AttachmentPreview, error) {
	s.mu.Lock()
	root := s.cfg.Paths.Workspace
	paths := append([]string(nil), s.opts.ContextPaths...)
	if s.runtime != nil {
		paths = s.runtime.ContextPaths()
	}
	limit := min(s.cfg.Int("limits.attachment_bytes"), s.cfg.Int("limits.context_tokens")*4)
	s.mu.Unlock()
	items, err := contextfiles.Previews(ctx, root, paths, limit)
	if err != nil {
		return nil, err
	}
	out := make([]ui.AttachmentPreview, 0, len(items))
	for _, item := range items {
		out = append(out, ui.AttachmentPreview{
			Path: item.Path, Kind: item.Kind, MIMEType: item.MIMEType,
			Bytes: item.Bytes, Width: item.Width, Height: item.Height, Data: item.Data,
		})
	}
	return out, nil
}
func (s *chatSession) ModelChoices(ctx context.Context) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.String("provider") == "" {
		return nil, errors.New("connect a provider with /login first")
	}
	catalog, err := s.providerCatalog()
	if err != nil {
		return nil, err
	}
	return core.ModelChoices(ctx, s.cfg, s.app.credentialLookup(s.cfg), catalog)
}

func (s *chatSession) providerCatalog() (*providercatalog.Catalog, error) {
	if s.opts.ProviderCatalog != nil {
		return s.opts.ProviderCatalog, nil
	}
	return core.DefaultCatalog(s.cfg.CustomProviders...)
}

func (s *chatSession) ModelProviders() []ui.ProviderOption {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.cfg.String("provider")
	catalog, err := s.providerCatalog()
	if err != nil {
		return nil
	}
	options := make([]ui.ProviderOption, 0, len(catalog.Entries()))
	for _, entry := range catalog.Entries() {
		options = append(options, ui.ProviderOption{ID: entry.ID, Name: entry.Name, Description: entry.Description, Current: entry.ID == current})
	}
	return options
}

func (s *chatSession) ModelCatalog(ctx context.Context, provider string) ([]ui.ModelOption, error) {
	s.mu.Lock()
	cfg := s.cfg
	cfg.Values = cloneValues(s.cfg.Values)
	currentProvider, currentModel := s.cfg.String("provider"), s.cfg.String("model")
	s.mu.Unlock()
	if provider == "" {
		provider = currentProvider
	}
	providers, err := s.providerCatalog()
	if err != nil {
		return nil, err
	}
	cfg.Values["provider"] = config.Value{Value: provider, Source: "interactive"}
	if providers.FixedEndpoint(provider) {
		cfg.Values["base_url"] = config.Value{Value: "", Source: "interactive"}
	}
	catalogCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	catalog, err := core.ModelCatalog(catalogCtx, cfg, s.app.credentialLookup(cfg), providers)
	if err != nil {
		return nil, err
	}
	values := make([]ui.ModelOption, 0, len(catalog))
	for _, item := range catalog {
		detail := "Reasoning unavailable"
		if item.Reasoning {
			detail = "Reasoning supported"
		}
		if provider != currentProvider || item.ID != currentModel {
			detail = "Available · " + detail
		} else {
			detail = "Currently selected · " + detail
		}
		values = append(values, ui.ModelOption{Provider: provider, ID: item.ID, Description: detail, Current: provider == currentProvider && item.ID == currentModel})
	}
	return values, nil
}

func (s *chatSession) Settings() []ui.SettingItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	value := func(key string) string {
		if v, ok := s.cfg.Values[key].Value.(string); ok {
			return v
		}
		return s.cfg.String(key)
	}
	thinking := s.opts.Thinking
	if thinking == "" {
		thinking = value("thinking")
	}
	contextPaths := append([]string(nil), s.opts.ContextPaths...)
	if s.runtime != nil {
		contextPaths = s.runtime.ContextPaths()
	}
	contextValue := "No additional files"
	if len(contextPaths) > 0 {
		contextValue = strings.Join(contextPaths, ", ")
	}
	items := []ui.SettingItem{
		{Key: "provider", Value: value("provider"), Source: settingSource(s.cfg, "provider"), Scope: "workspace", Description: "Provider for new executions"},
		{Key: "model", Value: value("model"), Source: settingSource(s.cfg, "model"), Scope: "workspace", Description: "Model used for new executions"},
		{Key: "web-search", Label: "Web search", Value: value("search.provider"), DisplayValue: webSearchSelectionLabel(value("search.provider")), Source: settingSource(s.cfg, "search.provider"), Scope: "user workspace", Description: "Choose the web search backend and configure its defaults"},
		{Key: "thinking", Value: thinking, Source: settingSource(s.cfg, "thinking"), Scope: "workspace", Description: "Reasoning level; supported by the selected model"},
		{Key: "ui.theme", Value: value("ui.theme"), Source: settingSource(s.cfg, "ui.theme"), Scope: "user workspace", Description: "Color theme for the interactive terminal"},
		{Key: "policy", Value: value("policy"), Source: settingSource(s.cfg, "policy"), Scope: "workspace", Description: "Tool permission policy; trusted allows effects without per-operation approval"},
		{Key: "context", Value: contextValue, Source: "current session", Scope: "session", Description: "UTF-8 files attached to requests in this chat"},
	}
	for _, path := range contextPaths {
		items = append(items, ui.SettingItem{Key: "context.remove:" + path, Value: path, Source: "current session", Scope: "session", Description: "Remove this attachment from future requests"})
	}
	return items
}

func webSearchSelectionLabel(selection string) string {
	switch selection {
	case "":
		return "Automatic"
	case "native":
		return "Hosted search for the active model provider"
	case "brave":
		return "Brave Search"
	case "disabled":
		return "Disabled"
	default:
		return selection
	}
}

func (s *chatSession) WebSearchProviders() []ui.WebSearchProviderOption {
	s.mu.Lock()
	defer s.mu.Unlock()
	catalog, err := s.providerCatalog()
	if err != nil {
		return nil
	}
	providers, current := core.WebSearchChoices(s.cfg, catalog)
	options := make([]ui.WebSearchProviderOption, 0, len(providers))
	for _, provider := range providers {
		options = append(options, ui.WebSearchProviderOption{ID: provider.ID, Name: provider.Name, Description: provider.Description, Current: provider.ID == current})
	}
	return options
}

func (s *chatSession) WebSearchSettings(providerID string) []ui.SettingItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	options := config.WebSearchOptions(providerID)
	items := make([]ui.SettingItem, 0, len(options))
	for _, option := range options {
		value := ""
		switch configured := s.cfg.Values[option.Key].Value.(type) {
		case string:
			value = configured
		case int:
			value = strconv.Itoa(configured)
		}
		displayValue := ""
		if value == "" || (strings.HasSuffix(option.Key, ".max_results") && value == "0") {
			displayValue = "Provider default"
		}
		items = append(items, ui.SettingItem{Key: option.Key, Label: option.Label, Value: value, DisplayValue: displayValue, InputKind: option.InputKind, Choices: append([]string(nil), option.Choices...), Source: settingSource(s.cfg, option.Key), Scope: "user workspace", Description: option.Description})
	}
	return items
}

func settingSource(cfg config.Resolved, key string) string {
	if source := cfg.Values[key].Source; source != "" {
		return source
	}
	return "default"
}

func cloneValues(values map[string]config.Value) map[string]config.Value {
	cloned := make(map[string]config.Value, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func (s *chatSession) SetSetting(ctx context.Context, key, value string) (string, error) {
	s.mu.Lock()
	source := s.cfg.Values[key].Source
	if source == "project" || source == "environment" || source == "flag" {
		s.mu.Unlock()
		return "", fmt.Errorf("%s is controlled by %s configuration", key, source)
	}
	s.mu.Unlock()
	if key == "search.provider" || strings.HasPrefix(key, "search.options.") {
		return s.setWebSearchPreference(ctx, key, value)
	}
	switch key {
	case "ui.theme":
		s.mu.Lock()
		paths := s.cfg.Paths
		s.mu.Unlock()
		if err := config.SaveWorkspacePreference(paths, key, value); err != nil {
			return "", err
		}
		s.mu.Lock()
		s.cfg.Values[key] = config.Value{Value: value, Source: "user-workspace"}
		s.mu.Unlock()
		return "Interface theme updated for this workspace.", nil
	case "thinking":
		s.mu.Lock()
		paths := s.cfg.Paths
		runtime := s.runtime
		s.mu.Unlock()
		message := "Reasoning level updated for this chat."
		if runtime != nil {
			var err error
			message, err = s.Command(ctx, "/thinking "+value)
			if err != nil {
				return "", err
			}
		}
		if err := config.SaveWorkspacePreference(paths, "thinking", value); err != nil {
			return "", fmt.Errorf("thinking changed for this chat, but workspace settings could not be saved: %w", err)
		}
		s.mu.Lock()
		s.opts.Thinking = value
		s.cfg.Values["thinking"] = config.Value{Value: value, Source: "user-workspace"}
		s.mu.Unlock()
		return message, nil
	case "policy":
		s.mu.Lock()
		if s.runtime != nil && s.runtime.Service.Active() {
			s.mu.Unlock()
			return "", errors.New("policy changes require an idle execution")
		}
		if err := config.ValidateWorkspaceSelection(s.cfg.String("provider"), s.cfg.String("model"), value); err != nil {
			s.mu.Unlock()
			return "", err
		}
		paths := s.cfg.Paths
		runtime := s.runtime
		old := s.cfg.String("policy")
		s.mu.Unlock()
		if err := config.SaveWorkspacePreference(paths, "policy", value); err != nil {
			return "", err
		}
		if runtime != nil {
			if err := runtime.Close(ctx); err != nil {
				_ = config.SaveWorkspacePreference(paths, "policy", old)
				return "", err
			}
		}
		s.mu.Lock()
		s.runtime = nil
		s.cfg.Values["policy"] = config.Value{Value: value, Source: "user-workspace"}
		s.mu.Unlock()
		return fmt.Sprintf("Policy changed from %s to %s for this workspace. New tools use the selected policy.", old, value), nil
	case "context.add", "context.remove":
		s.mu.Lock()
		if s.runtime != nil && s.runtime.Service.Active() {
			s.mu.Unlock()
			return "", errors.New("context changes require an idle execution")
		}
		paths := append([]string(nil), s.opts.ContextPaths...)
		if s.runtime != nil {
			paths = s.runtime.ContextPaths()
		}
		root := s.cfg.Paths.Workspace
		limit := min(s.cfg.Int("limits.attachment_bytes"), s.cfg.Int("limits.context_tokens")*4)
		s.mu.Unlock()
		if key == "context.add" {
			if strings.TrimSpace(value) == "" {
				return "", errors.New("choose a workspace file to attach")
			}
			paths = append(paths, strings.TrimSpace(value))
		} else {
			filtered := paths[:0]
			for _, path := range paths {
				if path != value {
					filtered = append(filtered, path)
				}
			}
			paths = filtered
		}
		bundle, err := contextfiles.Load(ctx, root, paths, limit)
		if err != nil {
			return "", err
		}
		normalized := make([]string, 0, len(bundle.Sources))
		for _, source := range bundle.Sources {
			if source.Kind == "attachment" {
				normalized = append(normalized, source.Path)
			}
		}
		s.mu.Lock()
		s.opts.ContextPaths = normalized
		if s.runtime != nil {
			s.runtime.SetContextPaths(normalized)
		}
		s.mu.Unlock()
		return fmt.Sprintf("Context files for this session: %s", strings.Join(normalized, ", ")), nil
	default:
		return "", fmt.Errorf("setting %s cannot be changed here", key)
	}
}

func (s *chatSession) setWebSearchPreference(ctx context.Context, key, value string) (string, error) {
	parsed, err := config.ParseWorkspacePreference(key, value)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	runtime := s.runtime
	paths := s.cfg.Paths
	cfg := s.cfg
	cfg.Values = cloneValues(s.cfg.Values)
	if runtime != nil && runtime.Service.Active() {
		s.mu.Unlock()
		return "", errors.New("web search settings can only change while the execution is idle")
	}
	s.mu.Unlock()
	if key == "search.provider" && value == "native" {
		catalog, err := s.providerCatalog()
		if err != nil {
			return "", err
		}
		if !catalog.HasNativeSearch(cfg.String("provider"), cfg) {
			return "", fmt.Errorf("hosted web search is unavailable for provider %q", cfg.String("provider"))
		}
	}
	if runtime != nil {
		if err := runtime.Close(ctx); err != nil {
			return "", err
		}
		s.mu.Lock()
		if s.runtime == runtime {
			s.runtime = nil
		}
		s.mu.Unlock()
	}
	if err := config.SaveWorkspacePreference(paths, key, value); err != nil {
		return "", err
	}
	s.mu.Lock()
	s.cfg.Values[key] = config.Value{Value: parsed, Source: "user-workspace"}
	s.mu.Unlock()
	return "Web search setting saved for this workspace. New searches will use the updated defaults.", nil
}

func (s *chatSession) inspectionService(ctx context.Context) (*execution.Service, func(), error) {
	s.mu.Lock()
	if s.runtime != nil {
		service := s.runtime.Service
		s.mu.Unlock()
		return service, func() {}, nil
	}
	cfg := s.cfg
	cfg.Values = cloneValues(s.cfg.Values)
	s.mu.Unlock()
	runtime, err := core.Open(ctx, cfg, core.Options{Profile: core.ProfileInspection})
	if err != nil {
		return nil, nil, err
	}
	return runtime.Service, func() {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = runtime.Close(closeCtx)
	}, nil
}

func (s *chatSession) SessionChoices(ctx context.Context) ([]ui.SessionSummary, error) {
	service, closeService, err := s.inspectionService(ctx)
	if err != nil {
		return nil, err
	}
	defer closeService()
	var entries []execution.SessionEntry
	for offset := 0; ; offset += 500 {
		page, err := service.Sessions(ctx, offset, 500)
		if err != nil {
			return nil, err
		}
		entries = append(entries, page...)
		if len(page) < 500 {
			break
		}
	}
	versions := map[string][]uint64{}
	documents, err := service.Records.List(ctx, "boundaries")
	if err != nil {
		return nil, err
	}
	for _, document := range documents {
		var boundary struct {
			RunID      string
			Checkpoint uint64
		}
		if json.Unmarshal(document.Data, &boundary) == nil && boundary.RunID != "" && boundary.Checkpoint > 0 {
			versions[boundary.RunID] = append(versions[boundary.RunID], boundary.Checkpoint)
		}
	}
	values := make([]ui.SessionSummary, 0, len(entries))
	for _, entry := range entries {
		metadata, err := service.Metadata(ctx, entry.ID)
		if err != nil {
			return nil, fmt.Errorf("load session %s: %w", entry.ID, err)
		}
		title := redactedPreview(metadata.InitialPrompt, 88)
		if title == "" {
			title = "Session " + entry.SessionID
		}
		preview := redactedPreview(metadata.InitialPrompt, 180)
		checkpointVersions := append([]uint64(nil), versions[entry.ID]...)
		if entry.Checkpoint > 0 {
			checkpointVersions = append(checkpointVersions, entry.Checkpoint)
		}
		sort.Slice(checkpointVersions, func(i, j int) bool { return checkpointVersions[i] < checkpointVersions[j] })
		checkpointVersions = uniqueVersions(checkpointVersions)
		values = append(values, ui.SessionSummary{
			ID: entry.ID, SessionID: entry.SessionID, Title: title, Preview: preview,
			Provider: metadata.Provider, Model: metadata.Model, State: entry.State, Parent: entry.Parent, Origin: metadata.Origin,
			Checkpoint: entry.Checkpoint, Checkpoints: checkpointVersions, Updated: entry.Updated,
		})
	}
	return values, nil
}

func (s *chatSession) ReadSession(ctx context.Context, id string, version uint64) (ui.SessionView, error) {
	service, closeService, err := s.inspectionService(ctx)
	if err != nil {
		return ui.SessionView{}, err
	}
	defer closeService()
	metadata, err := service.Metadata(ctx, id)
	if err != nil {
		return ui.SessionView{}, err
	}
	snapshot, err := service.Store.Load(ctx, id)
	if err != nil {
		return ui.SessionView{}, err
	}
	checkpoint := snapshot.Version
	state := string(snapshot.Status)
	if version > 0 && version != snapshot.Version {
		document, err := service.Records.Get(ctx, "boundaries", fmt.Sprintf("%s:%d", id, version))
		if err != nil {
			return ui.SessionView{}, errors.New("checkpoint is no longer available")
		}
		var boundary execution.Boundary
		if err := json.Unmarshal(document.Data, &boundary); err != nil || boundary.RunID != id || boundary.Checkpoint != version {
			return ui.SessionView{}, errors.New("checkpoint record is inconsistent")
		}
		metadata = boundary.Metadata
		snapshot = boundary.Snapshot
		checkpoint = version
		state = string(snapshot.Status)
	} else if document, err := service.Records.Get(ctx, "sessions", id); err == nil {
		var indexed execution.SessionEntry
		if json.Unmarshal(document.Data, &indexed) == nil && indexed.State != "" {
			state = indexed.State
			if state == "running" || state == "queued" {
				state = "interrupted"
			}
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return ui.SessionView{}, err
	}
	entry := ui.SessionSummary{ID: id, SessionID: metadata.SessionID, Title: redactedPreview(metadata.InitialPrompt, 88), Preview: transcriptPreview(snapshot.Agent.Transcript), Provider: metadata.Provider, Model: metadata.Model, State: state, Origin: metadata.Origin, Checkpoint: checkpoint}
	if entry.Title == "" {
		entry.Title = "Session " + entry.SessionID
	}
	messages := make([]ui.HistoryMessage, 0, len(snapshot.Agent.Transcript))
	for index, message := range snapshot.Agent.Transcript {
		if message.Role != ai.RoleUser && message.Role != ai.RoleAssistant {
			continue
		}
		var text strings.Builder
		for _, block := range message.Content {
			if block.Type == ai.ContentText && block.Text != "" {
				text.WriteString(block.Text)
			}
		}
		if text.Len() == 0 {
			continue
		}
		role := "Opera"
		if message.Role == ai.RoleUser {
			role = "You"
		}
		messages = append(messages, ui.HistoryMessage{ID: fmt.Sprintf("history:%s:%d", id, index), Role: role, Text: contextfiles.RedactText(text.String())})
	}
	return ui.SessionView{Summary: entry, Messages: messages}, nil
}

func (s *chatSession) BranchSession(ctx context.Context, id string, version uint64) (string, error) {
	service, closeService, err := s.inspectionService(ctx)
	if err != nil {
		return "", err
	}
	defer closeService()
	return service.Fork(ctx, id, version)
}

func (s *chatSession) NewSession(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runtime == nil {
		return nil
	}
	if s.runtime.Service.Active() {
		return errors.New("resolve the active execution before starting a new session")
	}
	if err := s.runtime.Close(ctx); err != nil {
		return err
	}
	s.runtime = nil
	s.opts.ResumeID = ""
	return nil
}

func (s *chatSession) ForkSession(ctx context.Context, id string) (string, error) {
	service, closeService, err := s.inspectionService(ctx)
	if err != nil {
		return "", err
	}
	defer closeService()
	return service.ForkSummary(ctx, id)
}

func transcriptPreview(messages []ai.Message) string {
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message.Role != ai.RoleAssistant && message.Role != ai.RoleUser {
			continue
		}
		var text strings.Builder
		for _, block := range message.Content {
			if block.Type == ai.ContentText {
				text.WriteString(block.Text)
			}
		}
		if text.Len() > 0 {
			return redactedPreview(text.String(), 180)
		}
	}
	return ""
}

func redactedPreview(value string, limit int) string {
	value = strings.Join(strings.Fields(contextfiles.RedactText(value)), " ")
	runes := []rune(value)
	if len(runes) > limit {
		value = string(runes[:max(1, limit-1)]) + "…"
	}
	return value
}

func uniqueVersions(versions []uint64) []uint64 {
	if len(versions) < 2 {
		return versions
	}
	out := versions[:1]
	for _, version := range versions[1:] {
		if version != out[len(out)-1] {
			out = append(out, version)
		}
	}
	return out
}

func (s *chatSession) Start(ctx context.Context, request execution.Request) *execution.Execution {
	s.mu.Lock()
	defer s.mu.Unlock()
	if request.ID != "" {
		if s.runtime == nil {
			if err := s.openResumeRuntime(ctx, request.ID); err != nil {
				return failedExecution(err)
			}
		} else {
			metadata, err := s.runtime.Service.Metadata(ctx, request.ID)
			if err != nil {
				return failedExecution(err)
			}
			if metadata.Workspace != s.cfg.Paths.Workspace {
				return failedExecution(errors.New("checkpoint belongs to another workspace"))
			}
			if s.runtime.Service.Provider != metadata.Provider || s.runtime.Service.Model != metadata.Model ||
				!reflect.DeepEqual(s.runtime.Service.ResponseFormat, metadata.ResponseFormat) {
				if s.runtime.Service.Active() {
					return failedExecution(errors.New("cannot switch models while an execution is active"))
				}
				if err := s.openResumeRuntime(ctx, request.ID); err != nil {
					return failedExecution(err)
				}
			} else {
				s.runtime.SetContextPaths(metadata.ContextPaths)
				s.opts.ContextPaths = append([]string(nil), metadata.ContextPaths...)
			}
		}
	}
	if err := s.ensureRuntime(ctx); err != nil {
		return failedExecution(err)
	}
	return s.runtime.Start(ctx, request)
}

func (s *chatSession) AnswerQuestion(runID, toolCallID string, answers []execution.UserQuestionAnswer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runtime == nil || s.runtime.Service.QuestionBroker == nil {
		return errors.New("no active user question")
	}
	return s.runtime.Service.QuestionBroker.Answer(runID, toolCallID, answers)
}

func failedExecution(err error) *execution.Execution {
	updates := make(chan execution.Update, 1)
	updates <- execution.Update{Result: &execution.Result{State: "failed", ExitCode: 1}, Error: err}
	close(updates)
	return &execution.Execution{Updates: updates}
}

func (s *chatSession) openResumeRuntime(ctx context.Context, id string) error {
	cfg := s.cfg
	cfg.Values = cloneValues(s.cfg.Values)
	cfg.Values["provider"] = config.Value{Value: "", Source: "checkpoint"}
	cfg.Values["model"] = config.Value{Value: "", Source: "checkpoint"}
	opts := s.opts
	opts.ResumeID = id
	// The checkpoint's attachments and response format are authoritative for chat recovery.
	opts.ContextPaths = nil
	opts.ResponseFormat = nil
	runtime, err := core.Open(ctx, cfg, opts)
	if err != nil {
		return err
	}
	_, provider, model := runtime.SessionInfo()
	s.cfg.Values["provider"] = config.Value{Value: provider, Source: "checkpoint"}
	s.cfg.Values["model"] = config.Value{Value: model, Source: "checkpoint"}
	s.opts.ContextPaths = runtime.ContextPaths()
	previous := s.runtime
	s.runtime = runtime
	if previous != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := previous.Close(cleanup); err != nil {
			return fmt.Errorf("checkpoint runtime opened; previous composition cleanup: %w", err)
		}
	}
	return nil
}
func (s *chatSession) FollowUp(id, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runtime == nil {
		return errors.New("no execution is active")
	}
	return s.runtime.FollowUp(id, text)
}
func (s *chatSession) Steer(id, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runtime == nil {
		return errors.New("no execution is active")
	}
	return s.runtime.Steer(id, text)
}

func (s *chatSession) ensureRuntime(ctx context.Context) error {
	if s.cfg.String("provider") == "" || s.cfg.String("model") == "" {
		return errors.New("use /login to connect, then choose a model with /model")
	}
	if s.runtime != nil {
		return nil
	}
	r, err := core.Open(ctx, s.cfg, s.opts)
	if err == nil {
		s.runtime = r
	}
	return err
}

func (s *chatSession) Command(ctx context.Context, text string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return "", nil
	}
	if parts[0] == "/help" {
		return execution.CommandHelp(), nil
	}
	if parts[0] == "/settings" && s.runtime == nil {
		return fmt.Sprintf("Provider: %s\nModel: %s\nPolicy: %s\nUse /login to connect a provider.", s.cfg.String("provider"), s.cfg.String("model"), s.cfg.String("policy")), nil
	}
	if parts[0] == "/model" && len(parts) == 3 {
		candidate := s.cfg
		candidate.Values = make(map[string]config.Value, len(s.cfg.Values))
		for k, v := range s.cfg.Values {
			candidate.Values[k] = v
		}
		candidate.Values["provider"] = config.Value{Value: parts[1], Source: "interactive"}
		candidate.Values["model"] = config.Value{Value: parts[2], Source: "interactive"}
		providers, err := s.providerCatalog()
		if err != nil {
			return "", err
		}
		if !providers.Has(parts[1]) {
			return "", fmt.Errorf("provider %q is not registered", parts[1])
		}
		if providers.FixedEndpoint(parts[1]) {
			candidate.Values["base_url"] = config.Value{Value: "", Source: "interactive"}
		}
		// Validate the selection before changing the runtime.
		if err := config.ValidateWorkspaceSelection(parts[1], parts[2], candidate.String("policy")); err != nil {
			return "", err
		}
		if s.runtime != nil {
			if _, err := s.runtime.Command(ctx, text); err != nil {
				return "", err
			}
		}
		s.cfg = candidate
		if err := config.SaveWorkspaceSelection(candidate.Paths, parts[1], parts[2], candidate.String("policy")); err != nil {
			return "", fmt.Errorf("model selected, but workspace settings could not be saved: %w", err)
		}
		return "Model selected. Ready to chat.", nil
	}
	if s.runtime == nil {
		if err := s.ensureRuntime(ctx); err != nil {
			return "", err
		}
	}
	value, err := s.runtime.Command(ctx, text)
	if err == nil && parts[0] == "/thinking" && len(parts) == 2 {
		s.opts.Thinking = parts[1]
		s.cfg.Values["thinking"] = config.Value{Value: parts[1], Source: "interactive"}
	}
	return value, err
}

func (s *chatSession) Close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runtime != nil {
		return s.runtime.Close(ctx)
	}
	return nil
}

func (a App) interactiveChat(ctx context.Context, cfg config.Resolved, opts core.Options, request execution.Request) (code int) {
	session := newChatSession(a, cfg, opts, nil)
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := session.Close(cleanup); err != nil {
			code = a.fail(1, err, false)
		}
	}()
	if cfg.Bool("ui.accessible") {
		return a.accessibleChat(ctx, session)
	}
	code, err := ui.Chat(ctx, session, a.In, a.Out, request, cfg.Bool("ui.no_motion"))
	if err != nil {
		return a.fail(1, err, false)
	}
	return code
}

func openLoginBrowser(ctx context.Context, u string) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		command, args = "open", []string{u}
	case "windows":
		command, args = "rundll32", []string{"url.dll,FileProtocolHandler", u}
	default:
		command, args = "xdg-open", []string{u}
	}
	return exec.CommandContext(ctx, command, args...).Run()
}
