package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateImportEnforcesArchitectureBoundaries(t *testing.T) {
	cases := []struct {
		path, imported string
		wantError      bool
	}{
		{"internal/modules/execution/service.go", "github.com/diasYuri/y/pkg/agent", false},
		{"internal/modules/execution/service.go", "github.com/diasYuri/opera/internal/adapters/providers/openai", true},
		{"internal/modules/execution/service.go", "github.com/diasYuri/opera/internal/core", true},
		{"internal/modules/extensions/manager.go", "github.com/diasYuri/opera/internal/modules/configuration", false},
		{"internal/modules/accounting/accounting.go", "github.com/diasYuri/opera/internal/adapters/persistence/sqlite", true},
		{"internal/adapters/persistence/sqlite/store.go", "github.com/diasYuri/opera/internal/core", true},
		{"internal/adapters/credentials/store.go", "github.com/diasYuri/opera/internal/ui", true},
		{"internal/core/open.go", "github.com/diasYuri/y/pkg/providers", false},
		{"internal/core/open.go", "github.com/diasYuri/y/pkg/providers/openai", false},
		{"internal/adapters/providers/openai/entry.go", "github.com/diasYuri/y/pkg/providers/openai", false},
		{"internal/core/component_provider.go", "github.com/diasYuri/opera/internal/adapters/providers/openai", false},
		{"internal/ui/cli/oauth.go", "github.com/diasYuri/opera/internal/adapters/providers/custom", true},
		{"internal/ui/tui/chat.go", "github.com/diasYuri/opera/internal/adapters/providers/openai", true},
		{"internal/ui/tui/autocomplete.go", "github.com/diasYuri/opera/internal/adapters/skills/filesystem", true},
		{"internal/ui/tui/autocomplete.go", "github.com/diasYuri/opera/internal/modules/skills/catalog", false},
		{"internal/ui/tui/attachments.go", "os/exec", true},
		{"internal/ui/tui/chat.go", "github.com/diasYuri/y/pkg/agent", true},
		{"internal/ui/tui/chat.go", "github.com/diasYuri/opera/internal/ui/cli", true},
		{"internal/ui/cli/execution.go", "github.com/diasYuri/opera/internal/core", false},
		{"internal/ui/cli/app.go", "github.com/diasYuri/opera/internal/adapters/credentials", false},
		{"internal/ui/cli/provider_login.go", "github.com/diasYuri/opera/internal/adapters/providers/openai", true},
		{"internal/ui/cli/provider_login.go", "github.com/diasYuri/opera/internal/adapters/providers/anthropic", true},
		{"internal/ui/cli/oauth.go", "github.com/diasYuri/opera/internal/adapters/providers/openrouter", true},
		{"internal/ui/cli/chatgpt.go", "github.com/diasYuri/opera/internal/adapters/providers/openai/subscription", true},
		{"internal/ui/cli/execution.go", "github.com/diasYuri/y/pkg/tools", true},
		{"internal/ui/cli/execution.go", "github.com/diasYuri/y/pkg/agent", true},
		{"internal/modules/execution/action.go", "github.com/diasYuri/y/pkg/internal/secret", true},
	}
	for _, test := range cases {
		err := validateImport(test.path, test.imported)
		if (err != nil) != test.wantError {
			t.Errorf("validateImport(%q, %q) error = %v", test.path, test.imported, err)
		}
	}
}

func TestCLIProviderOAuthProtocolsStayInAdapters(t *testing.T) {
	for _, detail := range []string{"claude.ai/oauth/authorize", "platform.claude.com/v1/oauth/token", "https://openrouter.ai/auth", "grant_type"} {
		if err := validateCLIProviderAuth("internal/ui/cli/provider_login.go", "package cli\nconst value = \""+detail+"\""); err == nil {
			t.Errorf("provider protocol detail %q was accepted in CLI", detail)
		}
	}
	if err := validateCLIProviderAuth("internal/ui/cli/oauth.go", "package cli\nconst endpoint = \"OPERA_OAUTH_TOKEN_URL\""); err != nil {
		t.Fatalf("generic OAuth configuration was rejected: %v", err)
	}
	for _, path := range []string{"internal/ui/cli/provider_login.go", "internal/ui/cli/chat_session.go", "internal/ui/cli/provider_auth.go", "internal/core/provider_catalog.go", "internal/core/provider_oauth.go"} {
		if err := validateCLIProviderAuth(path, "package boundary\nconst protocol = \"https://platform.claude.com/v1/oauth/token\""); err == nil {
			t.Errorf("provider protocol detail was accepted in %s", path)
		}
	}
}

func TestExternalIntegrationTestsMayComposeConcreteAdapters(t *testing.T) {
	sqlite := "github.com/diasYuri/opera/internal/adapters/persistence/sqlite"
	provider := "github.com/diasYuri/opera/internal/adapters/providers/openai/subscription"
	if err := validateImportForFile("internal/modules/accounting/accounting_test.go", sqlite, true); err != nil {
		t.Fatalf("external accounting test could not compose SQLite: %v", err)
	}
	if err := validateImportForFile("internal/modules/accounting/accounting.go", sqlite, false); err == nil {
		t.Fatal("accounting production code imported SQLite directly")
	}
	if err := validateImportForFile("internal/ui/cli/oauth_test.go", provider, true); err != nil {
		t.Fatalf("external CLI test could not compose a provider: %v", err)
	}
	if err := validateImportForFile("internal/ui/cli/provider_auth.go", provider, false); err == nil {
		t.Fatal("CLI production code imported a concrete provider")
	}
	agenttest := "github.com/diasYuri/y/pkg/agent/agenttest"
	if err := validateImportForFile("internal/ui/cli/execution_test.go", agenttest, true); err != nil {
		t.Fatalf("external CLI test could not use the public SDK test double: %v", err)
	}
	if err := validateImportForFile("internal/ui/cli/execution.go", agenttest, false); err == nil {
		t.Fatal("CLI production code imported the SDK agent test package")
	}
}

func TestExistingCoreAndCLIContainNoProviderProtocolDetails(t *testing.T) {
	for _, dir := range []string{"../internal/core", "../internal/ui/cli"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range files {
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateCLIProviderAuth(strings.TrimPrefix(filepath.ToSlash(path), "../"), string(source)); err != nil {
				t.Error(err)
			}
		}
	}
}
