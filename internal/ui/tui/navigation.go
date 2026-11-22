package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	"github.com/diasYuri/opera/internal/modules/execution"
)

// SessionSummary is the durable identity and preview shown by the session and
// checkpoint pickers. It contains no execution capability or approval token.
type SessionSummary struct {
	ID          string
	SessionID   string
	Title       string
	Preview     string
	Provider    string
	Model       string
	State       string
	Parent      string
	Origin      string
	Checkpoint  uint64
	Checkpoints []uint64
	Updated     time.Time
}

type HistoryMessage struct {
	ID, Role, Text string
}

type SessionView struct {
	Summary  SessionSummary
	Messages []HistoryMessage
}

type SessionBrowser interface {
	SessionChoices(context.Context) ([]SessionSummary, error)
	ReadSession(context.Context, string, uint64) (SessionView, error)
	BranchSession(context.Context, string, uint64) (string, error)
}

type SettingItem struct {
	Key, Label, Value, DisplayValue, Source, Scope, Description string
	InputKind                                                   string
	Choices                                                     []string
}

type SettingsManager interface {
	Settings() []SettingItem
	SetSetting(context.Context, string, string) (string, error)
}

type WebSearchSettingsManager interface {
	WebSearchProviders() []WebSearchProviderOption
	WebSearchSettings(string) []SettingItem
	WebSearchAuthOptions(string) []WebSearchAuthOption
	LoginWebSearch(context.Context, WebSearchAuthOption, func(execution.LoginNotice) error) error
}

type WebSearchProviderOption struct {
	ID, Name, Description string
	Current               bool
	AuthOptions           []WebSearchAuthOption
}

type WebSearchAuthOption struct {
	SearchProviderID, CredentialProviderID string
	MethodID, Kind, Name, Description      string
}

type ProviderOption struct {
	ID, Name, Description string
	Current               bool
}

type ModelOption struct {
	Provider, ID, Description string
	Current                   bool
}

func displayProvider(provider string) string {
	if provider == "openai-subscription" {
		return "openai"
	}
	return provider
}

type ModelCatalogProvider interface {
	ModelProviders() []ProviderOption
	ModelCatalog(context.Context, string) ([]ModelOption, error)
}

type sessionChoice struct{ SessionSummary }

func (c sessionChoice) Title() string {
	title := strings.TrimSpace(c.TitleText())
	if title == "" {
		title = c.ID
	}
	if c.Origin != "" {
		title = "↳ " + title
	}
	return safeText(fmt.Sprintf("%s · %s", title, c.State))
}
func (c sessionChoice) TitleText() string { return c.SessionSummary.Title }
func (c sessionChoice) Description() string {
	detail := strings.TrimSpace(c.Preview)
	if c.Origin != "" {
		detail += " · branched from " + c.Origin
	} else if c.Parent != "" {
		detail += " · parent " + c.Parent
	}
	if c.Checkpoint > 0 {
		detail += fmt.Sprintf(" · checkpoint %d", c.Checkpoint)
	}
	if c.Provider != "" || c.Model != "" {
		detail += " · " + displayProvider(c.Provider) + "/" + c.Model
	}
	if !c.Updated.IsZero() {
		detail += " · " + c.Updated.Local().Format("2006-01-02 15:04")
	}
	return safeText(detail)
}
func (c sessionChoice) FilterValue() string {
	return strings.Join([]string{c.Title(), c.SessionID, c.ID, c.State, c.Origin, c.Parent, c.Preview}, " ")
}

type checkpointChoice struct {
	SessionSummary
	Version uint64
}

func (c checkpointChoice) Title() string {
	return safeText(fmt.Sprintf("Checkpoint %d · %s", c.Version, c.State))
}
func (c checkpointChoice) Description() string {
	return safeText(fmt.Sprintf("Run %s · branch destination: new session from checkpoint %d", c.ID, c.Version))
}
func (c checkpointChoice) FilterValue() string {
	return fmt.Sprintf("%s %s %d %s", c.Title(), c.ID, c.Version, c.Preview)
}

type sessionActionChoice struct {
	key, title, description string
}

func (c sessionActionChoice) Title() string       { return safeText(c.title) }
func (c sessionActionChoice) Description() string { return safeText(c.description) }
func (c sessionActionChoice) FilterValue() string { return c.title + " " + c.description }

type settingChoice struct{ SettingItem }

func (c settingChoice) Title() string {
	label := c.Key
	if c.Label != "" {
		label = c.Label
	}
	value := c.Value
	if c.DisplayValue != "" {
		value = c.DisplayValue
	}
	if value == "" {
		return safeText(label)
	}
	return safeText(label + " · " + value)
}
func (c settingChoice) Description() string {
	return safeText(c.SettingItem.Description + " · " + c.Scope + " · from " + c.Source)
}
func (c settingChoice) FilterValue() string {
	return c.Key + " " + c.Label + " " + c.Value + " " + c.DisplayValue + " " + c.SettingItem.Description + " " + c.Source
}

type webSearchProviderChoice struct{ WebSearchProviderOption }

func (c webSearchProviderChoice) Title() string {
	title := c.Name
	if c.Current {
		title += " · selected"
	}
	return safeText(title)
}
func (c webSearchProviderChoice) Description() string {
	return safeText(c.WebSearchProviderOption.Description)
}
func (c webSearchProviderChoice) FilterValue() string {
	return c.ID + " " + c.Name + " " + c.WebSearchProviderOption.Description
}

type webSearchProviderAction struct {
	provider WebSearchProviderOption
	key      string
	title    string
	detail   string
}

func (c webSearchProviderAction) Title() string       { return safeText(c.title) }
func (c webSearchProviderAction) Description() string { return safeText(c.detail) }
func (c webSearchProviderAction) FilterValue() string {
	return c.provider.ID + " " + c.key + " " + c.title + " " + c.detail
}

type webSearchAuthChoice struct{ WebSearchAuthOption }

func (c webSearchAuthChoice) Title() string       { return safeText(c.Name) }
func (c webSearchAuthChoice) Description() string { return safeText(c.WebSearchAuthOption.Description) }
func (c webSearchAuthChoice) FilterValue() string {
	return c.Name + " " + c.WebSearchAuthOption.Description
}

type webSearchOptionChoice struct{ settingChoice }

type themeChoice struct {
	value, name, description string
	current                  bool
}

func (c themeChoice) Title() string {
	if c.current {
		return safeText(c.name + " · current")
	}
	return safeText(c.name)
}
func (c themeChoice) Description() string { return safeText(c.description) }
func (c themeChoice) FilterValue() string { return c.value + " " + c.name + " " + c.description }

type providerChoice struct{ ProviderOption }

func (c providerChoice) Title() string {
	if c.Current {
		return safeText(c.Name + " · selected")
	}
	return safeText(c.Name)
}
func (c providerChoice) Description() string { return safeText(c.ProviderOption.Description) }
func (c providerChoice) FilterValue() string {
	return c.Name + " " + c.ID + " " + c.ProviderOption.Description
}

type modelChoice struct{ ModelOption }

func (c modelChoice) Title() string {
	title := displayProvider(c.Provider) + " - " + c.ID
	if c.Current {
		title += " · selected"
	}
	return safeText(title)
}
func (c modelChoice) Description() string { return safeText(c.ModelOption.Description) }
func (c modelChoice) FilterValue() string {
	return displayProvider(c.Provider) + " " + c.Provider + " " + c.ID + " " + c.ModelOption.Description
}

type messageChoice struct {
	index int
	entry conversationEntry
}

func (c messageChoice) Title() string {
	return safeText(fmt.Sprintf("%s · %s", c.entry.state, strings.TrimSpace(c.entry.text)))
}
func (c messageChoice) Description() string { return fmt.Sprintf("Conversation message %d", c.index+1) }
func (c messageChoice) FilterValue() string { return c.entry.state + " " + c.entry.text }

var _ list.Item = sessionChoice{}
var _ list.Item = checkpointChoice{}
var _ list.Item = sessionActionChoice{}
var _ list.Item = settingChoice{}
var _ list.Item = webSearchProviderChoice{}
var _ list.Item = webSearchProviderAction{}
var _ list.Item = webSearchAuthChoice{}
var _ list.Item = webSearchOptionChoice{}
var _ list.Item = themeChoice{}
var _ list.Item = providerChoice{}
var _ list.Item = modelChoice{}
var _ list.Item = messageChoice{}

type sessionsMsg struct {
	values []SessionSummary
	err    error
	tree   bool
}

type sessionViewMsg struct {
	value SessionView
	err   error
	mode  string
}

type branchMsg struct {
	id      string
	version uint64
	err     error
}

type settingsMsg struct{ values []SettingItem }
type modelCatalogMsg struct {
	provider string
	values   []ModelOption
	err      error
}
type modelSelectionMsg struct{ err error }
type settingResultMsg struct {
	key, value, message string
	err                 error
}
