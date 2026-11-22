package core_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diasYuri/opera/internal/modules/configuration"
	execmod "github.com/diasYuri/opera/internal/modules/execution"
	"github.com/diasYuri/y/pkg/agent/agenttest"
)

func TestReadOnlyAgentCanUseBuiltinSkill(t *testing.T) {
	for _, request := range []execmod.Request{
		{RuntimeMode: execmod.RuntimeModePlan},
		{AgentProfile: execmod.AgentProfileReview},
	} {
		t.Run(request.RuntimeMode+request.AgentProfile, func(t *testing.T) {
			cfg := configuration(t)
			cfg.Values["policy"] = config.Value{Value: "read-only", Source: "test"}
			ctx := context.Background()
			runtime := open(t, cfg,
				agenttest.ToolTurn("skill_catalog", `{}`),
				agenttest.ToolTurn("skill_load", `{"name":"opera"}`),
				agenttest.ToolTurn("skill_resource", `{"name":"opera","path":"references/extend.md"}`),
				agenttest.TextTurn("Verified the extension contract."),
			)
			defer closeRuntime(t, runtime)
			request.Prompt = "How can I extend Opera?"
			result, err := runtime.Service.Execute(ctx, request, nil)
			if err != nil || result.ExitCode != 0 {
				t.Fatalf("skill execution failed: %+v %v", result, err)
			}
			state, err := runtime.Service.Store.Load(ctx, result.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]string{
				"skill_catalog":  `"source":"builtin"`,
				"skill_load":     "references/extend.md",
				"skill_resource": "Compiled providers, tools and components",
			}
			for _, message := range state.Agent.Transcript {
				out := message.ToolResult
				if out == nil || want[out.ToolName] == "" {
					continue
				}
				if out.IsError || len(out.Content) == 0 || !strings.Contains(out.Content[0].Text, want[out.ToolName]) {
					t.Fatalf("%s returned %+v", out.ToolName, out)
				}
				delete(want, out.ToolName)
			}
			if len(want) != 0 {
				t.Fatalf("missing tool results: %v", want)
			}
		})
	}
}

func TestBuiltinSkillUpgradePreservesPendingApproval(t *testing.T) {
	cfg := configuration(t)
	t.Setenv("HOME", cfg.Paths.Workspace)
	t.Setenv("USERPROFILE", cfg.Paths.Workspace)
	ctx := context.Background()
	runtime := open(t, cfg,
		agenttest.ToolTurn("write_file", `{"path":"result.txt","content":"approved"}`),
		agenttest.TextTurn("Done"),
	)
	defer closeRuntime(t, runtime)
	first, err := runtime.Service.Execute(ctx, execmod.Request{Prompt: "Write the result."}, nil)
	if err != nil || first.Approval == nil {
		t.Fatalf("missing pending approval: %+v %v", first, err)
	}
	metadata, err := runtime.Service.Metadata(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	newDigest := metadata.SkillGeneration
	metadata.SkillGeneration = "" // An empty catalog before the built-in skill.
	raw, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Service.Store.PutMetadata(ctx, "run:"+first.ID, raw); err != nil {
		t.Fatal(err)
	}

	resumed, err := runtime.Service.Execute(ctx, execmod.Request{ID: first.ID}, nil)
	if err != nil || resumed.Approval == nil || resumed.Approval.ID != first.Approval.ID {
		t.Fatalf("approval changed during skill upgrade: %+v %v", resumed, err)
	}
	metadata, err = runtime.Service.Metadata(ctx, first.ID)
	if err != nil || newDigest == "" || metadata.SkillGeneration != newDigest {
		t.Fatalf("skill generation was not migrated: %+v %v", metadata, err)
	}

	skillDir := filepath.Join(cfg.Paths.Workspace, ".agents", "skills", "other")
	if err := os.MkdirAll(skillDir, 0700); err != nil {
		t.Fatal(err)
	}
	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte("---\nname: other\ndescription: New skill\n---\nNew guidance"), 0600); err != nil {
		t.Fatal(err)
	}
	approve := execmod.Request{ID: first.ID, Approve: first.Approval.ID}
	if _, err := runtime.Service.Execute(ctx, approve, nil); err == nil || !strings.Contains(err.Error(), "skill catalog differs") {
		t.Fatalf("catalog drift should block approval: %v", err)
	}
	if err := os.Remove(skillFile); err != nil {
		t.Fatal(err)
	}
	final, err := runtime.Service.Execute(ctx, approve, nil)
	if err != nil || final.ExitCode != 0 {
		t.Fatalf("original approval could not be applied: %+v %v", final, err)
	}
}
