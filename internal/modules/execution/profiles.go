package execution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	AgentProfileDefault = "default"
	AgentProfileReview  = "reviewer"
)

// AgentProfile describes a specialist that can be invoked within a
// RuntimeMode. It contributes prompt guidance and can narrow the effective
// tool surface; it does not own an execution loop or grant permissions.
type AgentProfile struct {
	ID                  string
	Version             string
	Name                string
	Description         string
	Command             string
	PromptInstructions  string
	ContextInstructions string
	PromptPrefix        string
	DefaultTask         string
	Tools               ToolAccess
}

var agentProfileCatalog = []AgentProfile{
	{
		ID: AgentProfileDefault, Version: "1", Name: "Opera agent",
		Description: "General coding and workspace assistant",
		Tools:       ToolAccess{AllowMutations: true, AllowDelegation: true, AllowTaskManagement: true},
	},
	{
		ID: AgentProfileReview, Version: "1", Name: "Reviewer",
		Description:         "Review workspace changes and report actionable findings",
		Command:             "review",
		PromptInstructions:  "You are acting as a code reviewer. Prioritize concrete defects, regressions, security issues, and missing tests. Inspect relevant code and evidence before reporting. Do not modify files. Report findings first, ordered by severity, with file and line references; if you find none, say so and mention any validation limits.",
		ContextInstructions: "Use the current diff as primary evidence when available. Read surrounding implementation and relevant tests before drawing conclusions.",
		DefaultTask:         "Review the current workspace changes for bugs, regressions, and risky behavior.",
		Tools:               ToolAccess{AllowMutations: false, AllowDelegation: false, AllowTaskManagement: false},
	},
}

// ResolveAgentProfile returns a copy of a catalog profile. An empty ID selects
// the normal coding profile.
func ResolveAgentProfile(id string) (AgentProfile, error) {
	if id == "" {
		id = AgentProfileDefault
	}
	for _, profile := range agentProfileCatalog {
		if profile.ID == id {
			profile.Tools.AllowedTools = append([]string(nil), profile.Tools.AllowedTools...)
			profile.Tools.DeniedTools = append([]string(nil), profile.Tools.DeniedTools...)
			return profile, nil
		}
	}
	return AgentProfile{}, fmt.Errorf("unknown agent profile: %s", id)
}

func AvailableAgentProfiles() []AgentProfile {
	profiles := make([]AgentProfile, 0, len(agentProfileCatalog))
	for _, profile := range agentProfileCatalog {
		resolved, _ := ResolveAgentProfile(profile.ID)
		profiles = append(profiles, resolved)
	}
	return profiles
}

func (p AgentProfile) Digest() string { return definitionDigest(p) }

// ParseAgentProfileCommand recognizes profile slash commands such as
// `/review [task]`. Unrecognized commands are left to the normal command path.
func ParseAgentProfileCommand(text string) (profile AgentProfile, task string, recognized bool, err error) {
	text = strings.TrimSpace(text)
	fields := strings.Fields(text)
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/") {
		return AgentProfile{}, "", false, nil
	}
	command := strings.TrimPrefix(fields[0], "/")
	for _, candidate := range agentProfileCatalog {
		if candidate.Command == "" || candidate.Command != command {
			continue
		}
		profile, err = ResolveAgentProfile(candidate.ID)
		if err != nil {
			return AgentProfile{}, "", true, err
		}
		task = strings.TrimSpace(strings.TrimPrefix(text, fields[0]))
		if task == "" {
			task = profile.DefaultTask
		}
		return profile, task, true, nil
	}
	return AgentProfile{}, "", false, nil
}

func definitionDigest(value any) string {
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
