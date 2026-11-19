package execution

import (
	"context"
	"errors"
	"slices"

	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/ai"
)

// Runner is the application-facing subset of Y's runner. The public SDK owns
// execution; this port lets use cases depend on the contract rather than its
// concrete runner implementation.
type Runner interface {
	Run(context.Context, agent.RunRequest) (agent.RunResponse, error)
	Subscribe(func(agent.EventEnvelope)) func()
	FollowUp(string, ...ai.Message) error
	Steer(string, ...ai.Message) error
}

const (
	RuntimeModeDefault = "default"
	RuntimeModePlan    = "plan"
)

// OrchestrationProfile names the execution strategy selected by a RuntimeMode.
type OrchestrationProfile string

const (
	OrchestrationStandard OrchestrationProfile = "standard"
	OrchestrationPlanOnly OrchestrationProfile = "plan-only"
)

// ToolAccess describes tool capability constraints. An empty AllowedTools
// list leaves the runtime's normal registered tool set, subject to denials.
type ToolAccess struct {
	AllowedTools        []string
	DeniedTools         []string
	AllowMutations      bool
	AllowDelegation     bool
	AllowTaskManagement bool
}

// RuntimeMode describes how Opera executes an agent run. It is separate from
// AgentProfile, which describes the specialist performing work inside that run.
type RuntimeMode struct {
	ID                 string
	Version            string
	Name               string
	Description        string
	PromptInstructions string
	PromptPrefix       string
	Tools              ToolAccess
	Orchestration      OrchestrationProfile
}

// RuntimeModeOrchestrator adapts a concrete execution strategy to Y's public
// runner contract. The Y SDK continues to own the agent loop and lifecycle.
type RuntimeModeOrchestrator interface {
	Run(context.Context, Runner, agent.RunRequest) (agent.RunResponse, error)
}

type RuntimeModeOrchestratorFunc func(context.Context, Runner, agent.RunRequest) (agent.RunResponse, error)

func (f RuntimeModeOrchestratorFunc) Run(ctx context.Context, runner Runner, request agent.RunRequest) (agent.RunResponse, error) {
	return f(ctx, runner, request)
}

var runtimeModeCatalog = []RuntimeMode{
	{
		ID: RuntimeModeDefault, Version: "1", Name: "Default", Description: "Use the normal coding workflow",
		Tools:         ToolAccess{AllowMutations: true, AllowDelegation: true, AllowTaskManagement: true},
		Orchestration: OrchestrationStandard,
	},
	{
		ID: RuntimeModePlan, Version: "1", Name: "Plan", Description: "Inspect the workspace and propose a plan without implementing it",
		PromptInstructions: "PLAN MODE IS ACTIVE: inspect the workspace as needed and return a clear, actionable plan only. Do not modify files, run commands that change state, create tasks or delegate work. Do not begin implementation.",
		PromptPrefix:       "Plan the following request without implementing it:\n\n",
		Tools:              ToolAccess{AllowMutations: false, AllowDelegation: false, AllowTaskManagement: false},
		Orchestration:      OrchestrationPlanOnly,
	},
}

func (m RuntimeMode) Digest() string { return definitionDigest(m) }

func ResolveRuntimeMode(id string) (RuntimeMode, error) {
	if id == "" {
		id = RuntimeModeDefault
	}
	for _, mode := range runtimeModeCatalog {
		if mode.ID == id {
			mode.Tools.AllowedTools = slices.Clone(mode.Tools.AllowedTools)
			mode.Tools.DeniedTools = slices.Clone(mode.Tools.DeniedTools)
			return mode, nil
		}
	}
	return RuntimeMode{}, errors.New("unknown runtime mode: " + id)
}

func AvailableRuntimeModes() []RuntimeMode {
	modes := make([]RuntimeMode, 0, len(runtimeModeCatalog))
	for _, mode := range runtimeModeCatalog {
		resolved, _ := ResolveRuntimeMode(mode.ID)
		modes = append(modes, resolved)
	}
	return modes
}
