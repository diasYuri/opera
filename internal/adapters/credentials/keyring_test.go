package credentials

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

type boundedKeyring struct {
	values    map[string]string
	fail      func(string) bool
	beforeGet func(string)
	maxValue  int
}

func (k *boundedKeyring) Get(_, name string) (string, error) {
	if k.beforeGet != nil {
		k.beforeGet(name)
	}
	v, ok := k.values[name]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return v, nil
}
func (k *boundedKeyring) Set(_, name, value string) error {
	if k.fail != nil && k.fail(name) {
		return errors.New("native output must stay private")
	}
	if len(value) > 2000 {
		return keyring.ErrSetDataTooBig
	}
	if len(value) > k.maxValue {
		k.maxValue = len(value)
	}
	k.values[name] = value
	return nil
}
func (k *boundedKeyring) Delete(_, name string) error { delete(k.values, name); return nil }

func TestKeyringStoresOAuthSizedRecordsWithinNativeLimits(t *testing.T) {
	for _, secret := range []string{strings.Repeat("a", 2000), strings.Repeat("a", 2001), strings.Repeat("a", 65536), strings.Repeat("é界", 2000), keyringManifestPrefix + "literal-user-secret"} {
		backend := &boundedKeyring{values: map[string]string{}}
		store := systemStore{backend: backend}
		name := strings.Repeat("r", 100)
		if err := store.Set(name, secret); err != nil {
			t.Fatal("large OAuth record rejected", len(secret), err)
		}
		value, err := store.Get(name)
		if err != nil || value != secret {
			t.Fatal("keyring round trip changed the credential", len(secret), err)
		}
		for account, value := range backend.values {
			// Include go-keyring's base64 wrapper and conservative command overhead.
			commandBytes := len(account) + base64.StdEncoding.EncodedLen(len(value)) + 128
			if len(value) > 2560 || commandBytes > 4096 {
				t.Fatal("native entry still exceeds macOS/Windows limits")
			}
		}
		if err := store.Delete(name); err != nil {
			t.Fatal(err)
		}
		if len(backend.values) != 0 {
			t.Fatal("delete left credential parts behind")
		}
	}
}

func TestKeyringPreservesLegacyCredentialsAndCleansReplacedParts(t *testing.T) {
	backend := &boundedKeyring{values: map[string]string{"default": "legacy-api-key"}}
	store := systemStore{backend: backend}
	if value, err := store.Get("default"); err != nil || value != "legacy-api-key" {
		t.Fatal("legacy key no longer readable", err)
	}
	for _, value := range []string{strings.Repeat("first", 1600), strings.Repeat("second", 1800), "small-new-key"} {
		if err := store.Set("default", value); err != nil {
			t.Fatal(err)
		}
		if loaded, err := store.Get("default"); err != nil || loaded != value {
			t.Fatal("replacement lost credential", err)
		}
		if value == "small-new-key" && len(backend.values) != 1 {
			t.Fatal("old parts survived small replacement")
		}
	}
}

func TestKeyringFailedRotationPreservesPreviousCredential(t *testing.T) {
	for _, stage := range []string{"part", "manifest"} {
		t.Run(stage, func(t *testing.T) {
			backend := &boundedKeyring{values: map[string]string{}}
			store := systemStore{backend: backend}
			previous := strings.Repeat("previous", 1000)
			if err := store.Set("oauth", previous); err != nil {
				t.Fatal(err)
			}
			entries := len(backend.values)
			backend.fail = func(name string) bool {
				if stage == "manifest" {
					return name == "oauth"
				}
				return strings.HasSuffix(name, "-1")
			}
			if err := store.Set("oauth", strings.Repeat("replacement", 1000)); !errors.Is(err, ErrUnavailable) {
				t.Fatal("expected native write failure", err)
			}
			backend.fail = nil
			if value, err := store.Get("oauth"); err != nil || value != previous {
				t.Fatal("failed rotation destroyed previous credential", err)
			}
			if len(backend.values) != entries {
				t.Fatal("failed rotation leaked staged parts")
			}
		})
	}
}

func TestKeyringReadRetriesWhenRotationRemovesPreviousParts(t *testing.T) {
	backend := &boundedKeyring{values: map[string]string{}}
	store := systemStore{backend: backend}
	if err := store.Set("oauth", strings.Repeat("old", 2000)); err != nil {
		t.Fatal(err)
	}
	rotated := false
	newValue := strings.Repeat("new", 2000)
	backend.beforeGet = func(name string) {
		if !rotated && name != "oauth" {
			rotated = true
			if err := store.Set("oauth", newValue); err != nil {
				t.Fatal(err)
			}
		}
	}
	value, err := store.Get("oauth")
	if err != nil || value != newValue {
		t.Fatal("concurrent rotation was reported as missing/corrupt", err)
	}
}

func TestOversizedNativeEntryIsNotMisreportedAsKeyringAccessFailure(t *testing.T) {
	err := keyringError(keyring.ErrSetDataTooBig)
	if !errors.Is(err, ErrTooLarge) || errors.Is(err, ErrUnavailable) {
		t.Fatal("size limit was reported as unavailable keyring", err)
	}
}
