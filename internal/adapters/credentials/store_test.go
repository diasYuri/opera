package credentials

import (
	"errors"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestKeyringErrorsDoNotExposeNativeOutput(t *testing.T) {
	const secret = "native-secret-sentinel"
	if err := keyringError(errors.New(secret)); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatal("native keyring error was not redacted")
	}
	if !errors.Is(keyringError(errors.New(secret)), ErrUnavailable) {
		t.Fatal("keyring unavailability lost its recoverable classification")
	}
	if err := keyringError(keyring.ErrNotFound); !errors.Is(err, ErrNotFound) {
		t.Fatal("missing keyring entry lost its classification")
	}
	if err := keyringError(nil); err != nil {
		t.Fatal(err)
	}
}
