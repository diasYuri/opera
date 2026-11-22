package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/diasYuri/opera/internal/adapters/skills/filesystem"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/execution"
	"github.com/diasYuri/y/pkg/policy"
	"github.com/diasYuri/y/pkg/search"
	"github.com/diasYuri/y/pkg/tools"
)

// ToolPackContext contains only the services needed while registering a pack.
type ToolPackContext struct {
	Config    config.Resolved
	Policy    policy.Engine
	Limits    tools.ToolLimits
	Guard     *execution.Guard
	Questions *execution.QuestionBroker
	Skills    *skills.Manager
	Work      *execution.Work
	Service   *execution.Service
	Search    search.Provider
}

// ToolPack is an explicitly selected group of related tools.
type ToolPack struct {
	ID       string
	Register func(context.Context, *tools.Registry, ToolPackContext) error
}

func defaultToolPacks() []ToolPack {
	return []ToolPack{
		{ID: "filesystem", Register: func(_ context.Context, r *tools.Registry, c ToolPackContext) error {
			return tools.RegisterFilesystem(r, tools.FilesystemOptions{WorkspaceRoot: c.Config.Paths.Workspace, Policy: c.Policy, Limits: c.Limits})
		}},
		{ID: "shell", Register: func(_ context.Context, r *tools.Registry, c ToolPackContext) error {
			return tools.RegisterShell(r, tools.ShellOptions{WorkspaceRoot: c.Config.Paths.Workspace, Policy: c.Policy, Limits: c.Limits})
		}},
		{ID: "git", Register: func(_ context.Context, r *tools.Registry, c ToolPackContext) error {
			return tools.RegisterGit(r, tools.GitOptions{WorkspaceRoot: c.Config.Paths.Workspace, Policy: c.Policy, Limits: c.Limits})
		}},
		{ID: "code-search", Register: func(_ context.Context, r *tools.Registry, c ToolPackContext) error {
			return tools.RegisterCodeSearch(r, tools.CodeSearchOptions{WorkspaceRoot: c.Config.Paths.Workspace, Policy: c.Policy, Limits: c.Limits})
		}},
		{ID: "lsp", Register: func(_ context.Context, r *tools.Registry, c ToolPackContext) error {
			return tools.RegisterLSP(r, tools.LSPOptions{WorkspaceRoot: c.Config.Paths.Workspace, Policy: c.Policy, Limits: c.Limits})
		}},
		{ID: "skills", Register: func(_ context.Context, r *tools.Registry, c ToolPackContext) error { return c.Skills.Register(r) }},
		{ID: "session-context", Register: func(_ context.Context, r *tools.Registry, c ToolPackContext) error {
			if c.Service == nil {
				return nil
			}
			return c.Service.RegisterContextTools(r)
		}},
		{ID: "work", Register: func(_ context.Context, r *tools.Registry, c ToolPackContext) error { return c.Work.Register(r) }},
		{ID: "user-question", Register: func(_ context.Context, r *tools.Registry, c ToolPackContext) error { return c.Questions.Register(r) }},
		{ID: "web-search", Register: func(_ context.Context, r *tools.Registry, c ToolPackContext) error {
			if c.Search == nil {
				return nil
			}
			_, err := tools.RegisterWebSearchProvider(r, c.Search)
			return err
		}},
	}
}

func selectToolPacks(custom []ToolPack, disabled []string) ([]ToolPack, error) {
	all := append(defaultToolPacks(), custom...)
	selected := make(map[string]ToolPack, len(all))
	for _, pack := range all {
		if pack.ID == "" || pack.Register == nil {
			return nil, errors.New("tool pack requires ID and registration function")
		}
		if _, exists := selected[pack.ID]; exists {
			return nil, fmt.Errorf("duplicate tool pack %q", pack.ID)
		}
		selected[pack.ID] = pack
	}
	for _, id := range disabled {
		if _, ok := selected[id]; !ok {
			return nil, fmt.Errorf("tool pack %q is not registered", id)
		}
		delete(selected, id)
	}
	packs := make([]ToolPack, 0, len(selected))
	for _, pack := range all {
		if _, ok := selected[pack.ID]; ok {
			packs = append(packs, pack)
		}
	}
	return packs, nil
}

func installToolPacks(ctx context.Context, registry *tools.Registry, inputs ToolPackContext, packs []ToolPack) error {
	for _, pack := range packs {
		if err := pack.Register(ctx, registry, inputs); err != nil {
			return fmt.Errorf("register tool pack %q: %w", pack.ID, err)
		}
	}
	return nil
}
