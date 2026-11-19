// Package app owns execution use cases shared by chat, run and synchronous workers.
package execution

import (
	"encoding/json"
	"github.com/diasYuri/y/pkg/ai"
	"time"
)

type Event struct {
	ParentRunID   string          `json:"parent_run_id,omitempty"`
	SchemaVersion int             `json:"schema_version"`
	Sequence      uint64          `json:"sequence"`
	Type          string          `json:"type"`
	RunID         string          `json:"run_id"`
	SessionID     string          `json:"session_id,omitempty"`
	Timestamp     time.Time       `json:"timestamp"`
	Text          string          `json:"text,omitempty"`
	Tool          string          `json:"tool,omitempty"`
	Data          json.RawMessage `json:"data,omitempty"`
}
type Approval struct {
	TaskID      string          `json:"task_id,omitempty"`
	ID          string          `json:"id"`
	Agent       string          `json:"agent"`
	Tool        string          `json:"tool"`
	Arguments   json.RawMessage `json:"arguments"`
	Path        string          `json:"path,omitempty"`
	Diff        string          `json:"diff,omitempty"`
	Fingerprint string          `json:"fingerprint"`
	RuntimeID   string          `json:"runtime_approval_id"`
}
type Request struct {
	Management   bool
	newRun       bool
	Worker       bool
	RuntimeMode  string
	AgentProfile string
	ParentID     string
	ID           string
	Prompt       string
	ImagePaths   []string
	ContinueFrom string
	SessionID    string
	Approve      string
	Deny         string
}
type Result struct {
	StructuredOutput json.RawMessage `json:"structured_output,omitempty"`
	ID               string          `json:"run_id"`
	SessionID        string          `json:"session_id"`
	State            string          `json:"state"`
	ExitCode         int             `json:"exit_code"`
	Checkpoint       uint64          `json:"checkpoint"`
	Text             string          `json:"text,omitempty"`
	Approval         *Approval       `json:"approval,omitempty"`
}
type Metadata struct {
	ResponseFormat    *ai.ResponseFormat `json:"response_format,omitempty"`
	Management        bool               `json:"management,omitempty"`
	TaskID            string             `json:"task_id,omitempty"`
	ToolGeneration    string             `json:"tool_generation,omitempty"`
	SkillGeneration   string             `json:"skill_generation,omitempty"`
	SkillActivations  []string           `json:"skill_activations,omitempty"`
	Worker            bool               `json:"worker,omitempty"`
	RuntimeMode       string             `json:"mode,omitempty"`
	RuntimeModeDigest string             `json:"runtime_mode_digest,omitempty"`
	AgentProfile      string             `json:"agent_profile,omitempty"`
	ProfileDigest     string             `json:"agent_profile_digest,omitempty"`
	ParentID          string             `json:"parent_id,omitempty"`
	Origin            string             `json:"origin,omitempty"`
	OriginVersion     uint64             `json:"origin_version,omitempty"`
	ContextPaths      []string           `json:"context_paths,omitempty"`
	InitialPrompt     string             `json:"initial_prompt,omitempty"`
	Workspace         string             `json:"workspace"`
	Provider          string             `json:"provider"`
	Model             string             `json:"model"`
	SessionID         string             `json:"session_id"`
	HarnessVersion    string             `json:"harness_version"`
	Pending           *Approval          `json:"pending,omitempty"`
}
