package main

import "testing"

func TestValidateImportEnforcesArchitectureBoundaries(t *testing.T) {
	cases := []struct {
		path, imported string
		wantError      bool
	}{
		{"internal/app/service.go", "github.com/diasYuri/y/pkg/agent", false},
		{"internal/app/service.go", "github.com/diasYuri/y/pkg/storage/sqlite", true},
		{"internal/app/approval.go", "github.com/diasYuri/opera/internal/approvalfiles", true},
		{"internal/app/service.go", "github.com/diasYuri/opera/internal/contextfiles", true},
		{"internal/app/service.go", "github.com/diasYuri/opera/internal/contextdata", false},
		{"internal/app/service.go", "github.com/diasYuri/opera/internal/harness", true},
		{"internal/app/ports.go", "github.com/diasYuri/opera/internal/persistence", true},
		{"internal/app/service.go", "github.com/diasYuri/opera/internal/skills", true},
		{"internal/harness/open.go", "github.com/diasYuri/y/pkg/providers", false},
		{"internal/harness/open.go", "github.com/diasYuri/y/pkg/providers/openai", true},
		{"internal/provideradapters/native.go", "github.com/diasYuri/y/pkg/providers/openai", false},
		{"internal/ui/autocomplete.go", "github.com/diasYuri/opera/internal/skills", true},
		{"internal/ui/autocomplete.go", "github.com/diasYuri/opera/internal/skillcatalog", false},
		{"internal/ui/attachments.go", "os/exec", true},
		{"internal/ui/chat.go", "github.com/diasYuri/y/pkg/agent", true},
		{"internal/app/action.go", "github.com/diasYuri/y/pkg/internal/secret", true},
	}
	for _, test := range cases {
		err := validateImport(test.path, test.imported)
		if (err != nil) != test.wantError {
			t.Errorf("validateImport(%q, %q) error = %v", test.path, test.imported, err)
		}
	}
}
