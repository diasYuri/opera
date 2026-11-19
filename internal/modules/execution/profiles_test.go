package execution

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAgentProfileCommandResolvesSpecialistAndDefaultTask(t *testing.T) {
	profile, task, recognized, err := ParseAgentProfileCommand("/review")
	if err != nil || !recognized {
		t.Fatalf("review command: recognized=%v err=%v", recognized, err)
	}
	if profile.ID != AgentProfileReview || task != profile.DefaultTask {
		t.Fatalf("profile/task = %q/%q", profile.ID, task)
	}
	_, task, recognized, err = ParseAgentProfileCommand("/review inspect only the parser")
	if err != nil || !recognized || task != "inspect only the parser" {
		t.Fatalf("custom review task: recognized=%v task=%q err=%v", recognized, task, err)
	}
	if _, _, recognized, err := ParseAgentProfileCommand("/unknown task"); err != nil || recognized {
		t.Fatalf("unknown command was consumed: recognized=%v err=%v", recognized, err)
	}
}

func TestAgentProfileDigestIsStableAndChangesWithDefinition(t *testing.T) {
	one, err := ResolveAgentProfile(AgentProfileReview)
	if err != nil {
		t.Fatal(err)
	}
	two, err := ResolveAgentProfile(AgentProfileReview)
	if err != nil {
		t.Fatal(err)
	}
	if one.Digest() != two.Digest() {
		t.Fatal("same profile has different digests")
	}
	two.PromptInstructions += " Additional rule."
	if one.Digest() == two.Digest() {
		t.Fatal("changed profile has the same digest")
	}
}

func TestBuildPromptSeparatesRuntimeModeAndAgentProfile(t *testing.T) {
	mode, err := ResolveRuntimeMode(RuntimeModePlan)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := ResolveAgentProfile(AgentProfileReview)
	if err != nil {
		t.Fatal(err)
	}
	built := BuildPrompt(PromptInput{
		WorkspaceRoot:     "/workspace",
		RuntimeMode:       mode,
		AgentProfile:      profile,
		WorkspaceContext:  "<workspace-context>repo guidance</workspace-context>",
		SkillCatalog:      "available skills",
		SkillInstructions: "active skill instructions",
		Task:              "inspect the diff",
	})
	if built.UserPrompt != mode.PromptPrefix+profile.PromptPrefix+"inspect the diff" {
		t.Fatalf("user prompt = %q", built.UserPrompt)
	}
	ordered := []string{"You are Opera", mode.PromptInstructions, profile.PromptInstructions, profile.ContextInstructions, "repo guidance", "available skills", "active skill instructions"}
	previous := -1
	for _, part := range ordered {
		index := strings.Index(built.SystemPrompt, part)
		if index < 0 || index <= previous {
			t.Fatalf("prompt section %q missing or out of order", part)
		}
		previous = index
	}
	if len(built.Sections) != 7 {
		t.Fatalf("got %d prompt sections", len(built.Sections))
	}
	if empty := BuildPrompt(PromptInput{RuntimeMode: mode, AgentProfile: profile}); empty.UserPrompt != "" {
		t.Fatalf("empty task became prompt prefix %q", empty.UserPrompt)
	}
}

func TestMetadataReadsLegacyModeAsRuntimeMode(t *testing.T) {
	var metadata Metadata
	if err := json.Unmarshal([]byte(`{"mode":"plan"}`), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.RuntimeMode != RuntimeModePlan {
		t.Fatalf("legacy mode decoded as %q", metadata.RuntimeMode)
	}
}
