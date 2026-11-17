package diagnostics

import (
	"errors"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/y/pkg/extensions/wasm"
	"testing"
)

func TestDoctorDistinguishesCompiledHostWithoutStartingIt(t *testing.T) {
	cfg, err := config.Load(config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	report := Collect(cfg, func(string) (string, error) { return "", errors.New("missing") })
	expected := "disabled-by-build"
	if wasm.HostAvailable() {
		expected = "compiled"
	}
	for _, check := range report.Checks {
		if check.Name == "wasm" {
			if check.Status != expected {
				t.Fatal(check)
			}
			return
		}
	}
	t.Fatal("missing wasm build diagnostic")
}
