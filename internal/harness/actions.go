package harness

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/diasYuri/opera/internal/app"
	"github.com/diasYuri/opera/internal/config"
	"github.com/diasYuri/opera/internal/contextfiles"
)

// SessionInfo exposes only the current workspace and model for presentation.
func (r *Runtime) SessionInfo() (workspace, provider, model string) {
	return r.cfg.Paths.Workspace, r.Service.Provider, r.Service.Model
}

func (r *Runtime) Start(ctx context.Context, req app.Request) *app.Execution {
	return r.Service.Start(ctx, req)
}
func (r *Runtime) FollowUp(id, text string) error { return r.Service.FollowUp(id, text) }
func (r *Runtime) FollowUpWithImages(id, text string, paths []string) error {
	return r.Service.FollowUpWithImages(id, text, paths)
}
func (r *Runtime) Steer(id, text string) error { return r.Service.Steer(id, text) }

// RunShell executes an explicitly submitted composer command in the configured workspace.
func (r *Runtime) RunShell(ctx context.Context, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = r.cfg.Paths.Workspace
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func (r *Runtime) SetContextPaths(paths []string) {
	r.opts.ContextPaths = append([]string(nil), paths...)
	r.Service.ContextPaths = append([]string(nil), paths...)
}
func (r *Runtime) Command(ctx context.Context, text string) (string, error) {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return "", nil
	}
	if r.Service.Active() && parts[0] != "/tasks" && parts[0] != "/agents" && parts[0] != "/usage" && parts[0] != "/skills" {
		return "", fmt.Errorf("settings change requires an idle execution")
	}
	switch parts[0] {
	case "/skills":
		if r.Service.Skills == nil {
			return "No skills are available", nil
		}
		if !r.Service.Active() {
			r.Service.Skills.Refresh()
		}
		catalog := r.Service.Skills.Catalog()
		if len(parts) == 1 {
			raw, err := json.MarshalIndent(catalog, "", "  ")
			return string(raw), err
		}
		if len(parts) != 2 {
			return "", fmt.Errorf("usage: /skills [name]")
		}
		skill, ok := catalog.Find(parts[1])
		if !ok {
			return "", fmt.Errorf("unknown or invalid skill %q", parts[1])
		}
		return fmt.Sprintf("%s (%s)\n%s\n\n%s", skill.Name, skill.Source, skill.Description, skill.Body), nil
	case "/usage":
		if len(parts) != 2 {
			return "", fmt.Errorf("usage: /usage <run-id>")
		}
		report, err := r.Service.Usage(ctx, "run", parts[1], "")
		if err != nil {
			return "", err
		}
		raw, err := json.MarshalIndent(report, "", "  ")
		return string(raw), err
	case "/tools":
		raw, err := json.MarshalIndent(r.Extensions.Registry.List(), "", "  ")
		return string(raw), err
	case "/extensions", "/mcp", "/memory":
		if len(parts) > 1 && parts[0] == "/extensions" && parts[1] == "reload" {
			if err := r.Service.CheckReconfiguration(ctx, ""); err != nil {
				return "", err
			}
			if err := r.Extensions.Reload(ctx); err != nil {
				return "", err
			}
			descriptors, _ := json.Marshal(r.Extensions.Registry.List())
			r.Service.ToolGeneration = fmt.Sprintf("%x", sha256.Sum256(append(descriptors, []byte(r.Extensions.Digest())...)))
		}
		report, err := r.Extensions.Inspect(ctx)
		if err != nil {
			return "", err
		}
		raw, err := json.MarshalIndent(report, "", "  ")
		return string(raw), err
	case "/tasks", "/agents", "/compact":
		if len(parts) != 2 {
			return "", fmt.Errorf("%s requires a run ID", parts[0])
		}
		if _, err := r.Service.Metadata(ctx, parts[1]); err != nil {
			return "", err
		}
		var value any
		var err error
		switch parts[0] {
		case "/compact":
			err = r.Service.Compact(ctx, parts[1])
			value = "Compacted; the original checkpoint is archived"
		case "/tasks":
			value, err = r.Service.Work.Tasks(ctx, parts[1])
		case "/agents":
			value, err = r.Service.Work.Children(ctx, parts[1])
		}
		if err != nil {
			return "", err
		}
		raw, err := json.MarshalIndent(value, "", "  ")
		return string(raw), err
	case "/help":
		return app.CommandHelp(), nil
	case "/settings":
		return fmt.Sprintf("Provider: %s\nModel: %s\nThinking: %s\nPolicy: %s", r.Service.Provider, r.Service.Model, r.opts.Thinking, r.Service.Mode), nil
	case "/context":
		paths := append([]string(nil), r.opts.ContextPaths...)
		if len(parts) > 1 {
			paths = append(paths, strings.TrimSpace(strings.TrimPrefix(text, "/context")))
		}
		bundle, err := contextfiles.Load(ctx, r.cfg.Paths.Workspace, paths, r.Service.ContextBudget)
		if err != nil {
			return "", err
		}
		r.opts.ContextPaths = paths
		r.Service.ContextPaths = paths
		var out strings.Builder
		for _, source := range bundle.Sources {
			fmt.Fprintf(&out, "%s (%s, %d bytes)\n", source.Path, source.Kind, source.Bytes)
		}
		return out.String(), nil
	case "/model", "/thinking":
		cfg := r.cfg
		cfg.Values = map[string]config.Value{}
		for k, v := range r.cfg.Values {
			cfg.Values[k] = v
		}
		opts := r.opts
		opts.ResumeID = ""
		if parts[0] == "/model" {
			if len(parts) == 1 {
				return r.Service.Provider + " " + r.Service.Model + "\nUse /model <provider> <model>", nil
			}
			if len(parts) != 3 {
				return "", fmt.Errorf("usage: /model <provider> <model>")
			}
			cfg.Values["provider"] = config.Value{Value: parts[1], Source: "interactive"}
			cfg.Values["model"] = config.Value{Value: parts[2], Source: "interactive"}
		} else {
			if len(parts) != 2 {
				return "", fmt.Errorf("usage: /thinking off|minimal|low|medium|high|xhigh|max")
			}
			switch parts[1] {
			case "off", "minimal", "low", "medium", "high", "xhigh", "max":
				opts.Thinking = parts[1]
			default:
				return "", fmt.Errorf("unsupported thinking level")
			}
		}
		if r.opts.ProviderFactory == nil && opts.Thinking != "" && opts.Thinking != "off" {
			probe, err := providerFactory(ctx, cfg)
			if err != nil {
				return "", err
			}
			supported := probe.Capabilities(cfg.String("model")).Reasoning
			_ = probe.Close()
			if !supported {
				return "", fmt.Errorf("selected model does not advertise reasoning support")
			}
		}
		if err := r.Close(ctx); err != nil {
			return "", err
		}
		next, err := Open(ctx, cfg, opts)
		if err != nil {
			restored, restoreErr := Open(ctx, r.cfg, r.opts)
			if restoreErr == nil {
				*r = *restored
			}
			return "", err
		}
		*r = *next
		return "Selection applied at idle safe point", nil
	}
	return "", fmt.Errorf("unknown command; use /help")
}

func (r *Runtime) ModelChoices(ctx context.Context) ([]string, error) {
	return ModelChoices(ctx, r.cfg, r.opts.Credential)
}
