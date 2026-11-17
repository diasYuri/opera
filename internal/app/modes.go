package app

import (
	"context"
	"errors"
	"slices"

	"github.com/diasYuri/y/pkg/agent"
)

const (
	ModeDefault = "default"
	ModePlan    = "plan"
)

// OrchestrationProfile names the execution loop strategy a mode requests.
// The app layer owns the profile; runtime adapters can map it to runner options.
type OrchestrationProfile string

const (
	OrchestrationStandard OrchestrationProfile = "standard"
	OrchestrationPlanOnly OrchestrationProfile = "plan-only"
)

// ToolAccess describes a mode's tool surface. An empty AllowedTools list means
// the mode starts with the runtime's normal tool set, less explicit denials.
// This leaves room for modes to expose additional tools or curated subsets.
type ToolAccess struct {
	AllowedTools        []string
	DeniedTools         []string
	AllowMutations      bool
	AllowDelegation     bool
	AllowTaskManagement bool
}

// Mode is the declarative behavior profile selected for an agent run.
type Mode struct {
	ID                 string
	Name               string
	Description        string
	PromptInstructions string
	PromptPrefix       string
	Tools              ToolAccess
	Orchestration      OrchestrationProfile
}

// RunOrchestrator owns the agent loop for one orchestration profile. Tool
// policy and prompt composition stay in Service; profiles can choose a distinct
// loop without adding mode-specific branches to the UI or request handlers.
type RunOrchestrator interface {
	Run(context.Context, *agent.AgentRunner, agent.RunRequest) (agent.RunResponse, error)
}

type RunOrchestratorFunc func(context.Context, *agent.AgentRunner, agent.RunRequest) (agent.RunResponse, error)

func (f RunOrchestratorFunc) Run(ctx context.Context, runner *agent.AgentRunner, request agent.RunRequest) (agent.RunResponse, error) {
	return f(ctx, runner, request)
}

var modeCatalog = []Mode{
	{
		ID: ModeDefault, Name: "Default", Description: "Use the normal coding workflow",
		Tools:         ToolAccess{AllowMutations: true, AllowDelegation: true, AllowTaskManagement: true},
		Orchestration: OrchestrationStandard,
	},
	{
		ID: ModePlan, Name: "Plan", Description: "Inspect the workspace and propose a plan without implementing it",
		PromptInstructions: "PLAN MODE IS ACTIVE: inspect the workspace as needed and return a clear, actionable plan only. Do not modify files, run commands that change state, create tasks or delegate work. Do not begin implementation.",
		PromptPrefix:       "Plan the following request without implementing it:\n\n",
		Tools:              ToolAccess{AllowMutations: false, AllowDelegation: false, AllowTaskManagement: false},
		Orchestration:      OrchestrationPlanOnly,
	},
}

func ResolveMode(id string) (Mode, error) {
	if id == "" {
		id = ModeDefault
	}
	for _, mode := range modeCatalog {
		if mode.ID == id {
			mode.Tools.AllowedTools = slices.Clone(mode.Tools.AllowedTools)
			mode.Tools.DeniedTools = slices.Clone(mode.Tools.DeniedTools)
			return mode, nil
		}
	}
	return Mode{}, errors.New("unknown agent mode: " + id)
}

func AvailableModes() []Mode {
	modes := make([]Mode, 0, len(modeCatalog))
	for _, mode := range modeCatalog {
		resolved, _ := ResolveMode(mode.ID)
		modes = append(modes, resolved)
	}
	return modes
}
