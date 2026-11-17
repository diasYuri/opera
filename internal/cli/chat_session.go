package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/diasYuri/opera/internal/app"
	"github.com/diasYuri/opera/internal/chatgpt"
	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/opera/internal/contextfiles"
	"github.com/diasYuri/opera/internal/harness"
	"github.com/diasYuri/opera/internal/provideradapters"
	"github.com/diasYuri/opera/internal/providercatalog"
	"github.com/diasYuri/opera/internal/ui"
	"github.com/diasYuri/y/pkg/ai"
)

type loginMethod struct {
	app.LoginProvider
	provider string
	login    func(context.Context, config.Resolved, func(app.LoginNotice) error) error
}

// chatSession can show commands and sign-in before an execution runtime exists.
// UI actions are serialized; auth notices never write directly to the terminal.
type chatSession struct {
	mu          sync.Mutex
	app         App
	cfg         config.Resolved
	opts        harness.Options
	runtime     *harness.Runtime
	methods     []loginMethod
	openBrowser func(context.Context, string) error
}

func newChatSession(a App, cfg config.Resolved, opts harness.Options, r *harness.Runtime) *chatSession {
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
	s.methods = []loginMethod{{LoginProvider: app.LoginProvider{ID: "openai", Name: "OpenAI", Description: "Sign in with ChatGPT"}, provider: chatgpt.ProviderID, login: s.subscriptionLoginWithRecovery}}
	return s
}

func (s *chatSession) LoginProviders() []app.LoginProvider {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]app.LoginProvider, 0, len(s.methods))
	for _, method := range s.methods {
		out = append(out, method.LoginProvider)
	}
	return out
}

func (s *chatSession) Login(ctx context.Context, id string, notify func(app.LoginNotice) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, method := range s.methods {
		if method.ID != id {
			continue
		}
		if err := method.login(ctx, s.cfg, func(notice app.LoginNotice) error {
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
					return notify(app.LoginNotice{Message: "Open the authorization link above in your browser to continue."})
				}
			}
			return nil
		}); err != nil {
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
		s.cfg.Values["provider"] = config.Value{Value: method.provider, Source: "interactive"}
		// A model from another provider must never be reused after sign-in.
		s.cfg.Values["model"] = config.Value{Value: "", Source: "interactive"}
		s.cfg.Values["base_url"] = config.Value{Value: "", Source: "interactive"}
		return nil
	}
	return errors.New("login provider is unavailable; use /login to see available providers")
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
		paths = append([]string(nil), s.runtime.Service.ContextPaths...)
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
	return harness.ModelChoices(ctx, s.cfg, s.app.credentialLookup(s.cfg), catalog)
}

func (s *chatSession) providerCatalog() (*providercatalog.Catalog, error) {
	if s.opts.ProviderCatalog != nil {
		return s.opts.ProviderCatalog, nil
	}
	return provideradapters.DefaultCatalog()
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
	cfg.Values["provider"] = config.Value{Value: provider, Source: "interactive"}
	if provider == "openai-subscription" {
		cfg.Values["base_url"] = config.Value{Value: "", Source: "interactive"}
	}
	catalogCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	providers, err := s.providerCatalog()
	if err != nil {
		return nil, err
	}
	catalog, err := harness.ModelCatalog(catalogCtx, cfg, s.app.credentialLookup(cfg), providers)
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
		contextPaths = append([]string(nil), s.runtime.Service.ContextPaths...)
	}
	contextValue := "No additional files"
	if len(contextPaths) > 0 {
		contextValue = strings.Join(contextPaths, ", ")
	}
	items := []ui.SettingItem{
		{Key: "provider", Value: value("provider"), Source: settingSource(s.cfg, "provider"), Scope: "workspace", Description: "Provider for new executions"},
		{Key: "model", Value: value("model"), Source: settingSource(s.cfg, "model"), Scope: "workspace", Description: "Model used for new executions"},
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
			paths = append([]string(nil), s.runtime.Service.ContextPaths...)
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

func (s *chatSession) inspectionService(ctx context.Context) (*app.Service, func(), error) {
	s.mu.Lock()
	if s.runtime != nil {
		service := s.runtime.Service
		s.mu.Unlock()
		return service, func() {}, nil
	}
	cfg := s.cfg
	cfg.Values = cloneValues(s.cfg.Values)
	s.mu.Unlock()
	runtime, err := harness.Open(ctx, cfg, harness.Options{Inspection: true})
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
	var entries []app.SessionEntry
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
		var boundary app.Boundary
		if err := json.Unmarshal(document.Data, &boundary); err != nil || boundary.RunID != id || boundary.Checkpoint != version {
			return ui.SessionView{}, errors.New("checkpoint record is inconsistent")
		}
		metadata = boundary.Metadata
		snapshot = boundary.Snapshot
		checkpoint = version
		state = string(snapshot.Status)
	} else if document, err := service.Records.Get(ctx, "sessions", id); err == nil {
		var indexed app.SessionEntry
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

func (s *chatSession) Start(ctx context.Context, request app.Request) *app.Execution {
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
			if s.runtime.Service.Provider != metadata.Provider || s.runtime.Service.Model != metadata.Model {
				if s.runtime.Service.Active() {
					return failedExecution(errors.New("cannot switch models while an execution is active"))
				}
				if err := s.runtime.Close(ctx); err != nil {
					return failedExecution(err)
				}
				s.runtime = nil
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

func (s *chatSession) AnswerQuestion(runID, toolCallID string, answers []app.UserQuestionAnswer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runtime == nil || s.runtime.Service.QuestionBroker == nil {
		return errors.New("no active user question")
	}
	return s.runtime.Service.QuestionBroker.Answer(runID, toolCallID, answers)
}

func failedExecution(err error) *app.Execution {
	updates := make(chan app.Update, 1)
	updates <- app.Update{Result: &app.Result{State: "failed", ExitCode: 1}, Error: err}
	close(updates)
	return &app.Execution{Updates: updates}
}

func (s *chatSession) openResumeRuntime(ctx context.Context, id string) error {
	cfg := s.cfg
	cfg.Values = cloneValues(s.cfg.Values)
	cfg.Values["provider"] = config.Value{Value: "", Source: "checkpoint"}
	cfg.Values["model"] = config.Value{Value: "", Source: "checkpoint"}
	opts := s.opts
	opts.ResumeID = id
	// The checkpoint's context is authoritative when recovering that run.
	opts.ContextPaths = nil
	runtime, err := harness.Open(ctx, cfg, opts)
	if err != nil {
		return err
	}
	_, provider, model := runtime.SessionInfo()
	s.cfg.Values["provider"] = config.Value{Value: provider, Source: "checkpoint"}
	s.cfg.Values["model"] = config.Value{Value: model, Source: "checkpoint"}
	s.opts.ContextPaths = append([]string(nil), runtime.Service.ContextPaths...)
	s.runtime = runtime
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
	r, err := harness.Open(ctx, s.cfg, s.opts)
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
		return app.CommandHelp(), nil
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
		if parts[1] == "openai-subscription" {
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

func (a App) interactiveChat(ctx context.Context, cfg config.Resolved, opts harness.Options, request app.Request) (code int) {
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
