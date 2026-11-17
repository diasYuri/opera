// Package credentials stores secrets separately from configuration and events.
package credentials

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/zalando/go-keyring"
)

var ErrNotFound = errors.New("credential not found")
var ErrTooLarge = errors.New("credential exceeds the native keyring entry size limit")
var ErrUnavailable = errors.New("system keyring unavailable; unlock/configure it or explicitly select the file store with --store file")

type Store interface {
	Set(name, secret string) error
	Get(name string) (string, error)
	Delete(name string) error
}

func Open(kind, directory string) (Store, error) {
	switch kind {
	case "keyring":
		return systemStore{}, nil
	case "file":
		return FileStore{Directory: directory}, nil
	default:
		return nil, fmt.Errorf("unknown credential store")
	}
}

func validate(name string) error {
	if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,99}$`).MatchString(name) {
		return fmt.Errorf("credential reference must be 1-100 ASCII letters, digits, dots, underscores or hyphens, starting with a letter or digit")
	}
	return nil
}

func validateSecret(secret string) error {
	if len(secret) == 0 || len(secret) > 65536 {
		return fmt.Errorf("credential must contain 1-65536 bytes")
	}
	return nil
}

func keyringError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, keyring.ErrSetDataTooBig) {
		return ErrTooLarge
	}
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrNotFound
	}
	// Native keyring errors may contain subprocess output. Do not expose it.
	return ErrUnavailable
}
