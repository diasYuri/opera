package cli

import (
	"context"
	"errors"

	"charm.land/huh/v2"
	"github.com/diasYuri/opera/internal/core"
	"github.com/diasYuri/opera/internal/modules/configuration"
)

func (a App) onboard(ctx context.Context, cfg config.Resolved) (config.Resolved, error) {
	if !a.IsTTY {
		return cfg, errors.New("onboarding requires a TTY")
	}
	provider, model, policy := cfg.String("provider"), cfg.String("model"), cfg.String("policy")
	if provider == "" {
		provider = "openai"
	}
	providerCatalog, err := core.DefaultCatalog(cfg.CustomProviders...)
	if err != nil {
		return cfg, err
	}
	providerOptions := make([]huh.Option[string], 0, len(providerCatalog.Entries()))
	for _, entry := range providerCatalog.Entries() {
		name := entry.Name
		if entry.Login != nil {
			name = entry.Login.Name
		}
		providerOptions = append(providerOptions, huh.NewOption(name, entry.ID))
	}
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Provider").Options(providerOptions...).Value(&provider),
		huh.NewInput().Title("Model ID").Value(&model).Validate(func(value string) error {
			if value == "" {
				return errors.New("a model ID is required")
			}
			return nil
		}),
		huh.NewSelect[string]().Title("Workspace policy").Description("Ask is recommended. Project files cannot grant permissions.").Options(huh.NewOption("Ask (recommended)", "ask"), huh.NewOption("Read-only", "read-only"), huh.NewOption("Trusted", "trusted")).Value(&policy),
	)).WithInput(a.In).WithOutput(a.Out).WithHeight(16).WithAccessible(cfg.Bool("ui.accessible"))
	if err := form.RunWithContext(ctx); err != nil {
		return cfg, err
	}
	if err := config.SaveWorkspaceSelection(cfg.Paths, provider, model, policy); err != nil {
		return cfg, err
	}
	for key, value := range map[string]string{"provider": provider, "model": model, "policy": policy} {
		cfg.Values[key] = config.Value{Value: value, Source: "user-workspace"}
	}
	return cfg, nil
}
