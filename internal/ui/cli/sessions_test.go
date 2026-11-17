package cli_test

import (
	"github.com/diasYuri/opera/internal/adapters/credentials"
	"strings"
	"testing"
)

func TestSessionsInspectionDoesNotLoadProviderOrCredentials(t *testing.T) {
	f := newFixture(t)
	f.app.OpenStore = func(string, string) (credentials.Store, error) {
		t.Fatal("sessions opened credential store")
		return nil, nil
	}
	if code := f.app.Run([]string{"sessions", "list", "0", "1", "--format=jsonl"}); code != 0 {
		t.Fatal(code, f.err.String())
	}
	if !strings.Contains(f.out.String(), "sessions.list") {
		t.Fatal(f.out.String())
	}
	f.out.Reset()
	if code := f.app.Run([]string{"sessions", "inspect", "missing", "--format=jsonl"}); code == 0 {
		t.Fatal("unknown session accepted")
	}
	if strings.Contains(f.err.String(), "provider") {
		t.Fatal("inspection tried to configure a provider", f.err.String())
	}
}
