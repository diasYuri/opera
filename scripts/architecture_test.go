package main

import "testing"

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
		{"internal/cli/oauth.go", "github.com/diasYuri/opera/internal/adapters/providers/custom", true},
		{"internal/ui/chat.go", "github.com/diasYuri/opera/internal/adapters/providers/openai", true},
		{"internal/ui/autocomplete.go", "github.com/diasYuri/opera/internal/adapters/skills/filesystem", true},
		{"internal/ui/autocomplete.go", "github.com/diasYuri/opera/internal/modules/skills/catalog", false},
		{"internal/ui/attachments.go", "os/exec", true},
		{"internal/ui/chat.go", "github.com/diasYuri/y/pkg/agent", true},
		{"internal/modules/execution/action.go", "github.com/diasYuri/y/pkg/internal/secret", true},
	}
	for _, test := range cases {
		err := validateImport(test.path, test.imported)
		if (err != nil) != test.wantError {
			t.Errorf("validateImport(%q, %q) error = %v", test.path, test.imported, err)
		}
	}
}
