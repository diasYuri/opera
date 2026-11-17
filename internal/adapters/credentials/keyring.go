package credentials

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/zalando/go-keyring"
)

// go-keyring v0.2.8 limits macOS's base64-encoded security command to 4096
// bytes and Windows credential blobs to 2560 bytes. Split before calling Set:
// its macOS oversized-data path starts a subprocess before rejecting the input.
const keyringPartBytes = 2000
const keyringManifestPrefix = "opera-keyring-v1:"

type keyringBackend interface {
	Get(string, string) (string, error)
	Set(string, string, string) error
	Delete(string, string) error
}
type nativeKeyring struct{}

func (nativeKeyring) Get(service, name string) (string, error) { return keyring.Get(service, name) }
func (nativeKeyring) Set(service, name, value string) error    { return keyring.Set(service, name, value) }
func (nativeKeyring) Delete(service, name string) error        { return keyring.Delete(service, name) }

type systemStore struct{ backend keyringBackend }

func (s systemStore) driver() keyringBackend {
	if s.backend != nil {
		return s.backend
	}
	return nativeKeyring{}
}

type keyringManifest struct {
	Version string `json:"version"`
	Parts   int    `json:"parts"`
	Digest  string `json:"digest"`
}

func (m keyringManifest) partName(name string, index int) string {
	return fmt.Sprintf("%s.opera-%s-%d", name, m.Version, index)
}

func parseKeyringManifest(value string) (keyringManifest, bool, error) {
	var manifest keyringManifest
	if !strings.HasPrefix(value, keyringManifestPrefix) {
		return manifest, false, nil
	}
	if json.Unmarshal([]byte(strings.TrimPrefix(value, keyringManifestPrefix)), &manifest) != nil {
		return manifest, true, errors.New("invalid Opera keyring manifest")
	}
	version, err := hex.DecodeString(manifest.Version)
	if err != nil || len(version) != 16 || manifest.Parts < 1 || manifest.Parts > (65536+keyringPartBytes-1)/keyringPartBytes {
		return manifest, true, errors.New("invalid Opera keyring manifest")
	}
	digest, err := hex.DecodeString(manifest.Digest)
	if err != nil || len(digest) != sha256.Size {
		return manifest, true, errors.New("invalid Opera keyring manifest")
	}
	return manifest, true, nil
}

func (s systemStore) Set(name, secret string) error {
	if err := validate(name); err != nil {
		return err
	}
	if err := validateSecret(secret); err != nil {
		return err
	}
	driver := s.driver()
	previous, err := driver.Get("opera", name)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return keyringError(err)
	}
	old, oldChunked, _ := parseKeyringManifest(previous)
	if len(secret) <= keyringPartBytes && !strings.HasPrefix(secret, keyringManifestPrefix) {
		if err := driver.Set("opera", name, secret); err != nil {
			return keyringError(err)
		}
		if oldChunked {
			s.removeParts(name, old)
		}
		return nil
	}
	var version [16]byte
	_, _ = rand.Read(version[:])
	manifest := keyringManifest{Version: hex.EncodeToString(version[:]), Parts: (len(secret) + keyringPartBytes - 1) / keyringPartBytes, Digest: fmt.Sprintf("%x", sha256.Sum256([]byte(secret)))}
	committed := false
	defer func() {
		if !committed {
			s.removeParts(name, manifest)
		}
	}()
	for index := 0; index < manifest.Parts; index++ {
		part := secret[index*keyringPartBytes : min(len(secret), (index+1)*keyringPartBytes)]
		if err := driver.Set("opera", manifest.partName(name, index), part); err != nil {
			return keyringError(err)
		}
	}
	raw, _ := json.Marshal(manifest)
	// Publishing the manifest last leaves the old record usable on any failure.
	if err := driver.Set("opera", name, keyringManifestPrefix+string(raw)); err != nil {
		return keyringError(err)
	}
	committed = true
	if oldChunked {
		s.removeParts(name, old)
	}
	return nil
}

func (s systemStore) Get(name string) (string, error) {
	if err := validate(name); err != nil {
		return "", err
	}
	driver := s.driver()
	for attempt := 0; attempt < 3; attempt++ {
		root, err := driver.Get("opera", name)
		if err != nil {
			return "", keyringError(err)
		}
		manifest, chunked, err := parseKeyringManifest(root)
		if err != nil {
			return "", err
		}
		if !chunked {
			return root, nil
		}
		var value strings.Builder
		var readErr error
		for index := 0; index < manifest.Parts; index++ {
			part, err := driver.Get("opera", manifest.partName(name, index))
			if err != nil {
				readErr = err
				break
			}
			if len(part) > keyringPartBytes {
				return "", errors.New("invalid Opera keyring part")
			}
			value.WriteString(part)
		}
		if readErr != nil {
			// A writer may have published a new version and removed old parts.
			current, err := driver.Get("opera", name)
			if err != nil {
				return "", keyringError(err)
			}
			if current != root {
				continue
			}
			if errors.Is(readErr, keyring.ErrNotFound) {
				return "", errors.New("keyring credential is incomplete; sign in again")
			}
			return "", keyringError(readErr)
		}
		secret := value.String()
		if validateSecret(secret) != nil || fmt.Sprintf("%x", sha256.Sum256([]byte(secret))) != manifest.Digest {
			return "", errors.New("keyring credential failed integrity validation; sign in again")
		}
		return secret, nil
	}
	return "", errors.New("keyring credential changed during reads; retry the operation")
}

func (s systemStore) Delete(name string) error {
	if err := validate(name); err != nil {
		return err
	}
	driver := s.driver()
	root, err := driver.Get("opera", name)
	if err != nil {
		return keyringError(err)
	}
	manifest, chunked, _ := parseKeyringManifest(root)
	if err := driver.Delete("opera", name); err != nil {
		return keyringError(err)
	}
	if chunked {
		s.removeParts(name, manifest)
	}
	return nil
}
func (s systemStore) removeParts(name string, manifest keyringManifest) {
	// A malformed manifest must never expand cleanup beyond the bounded record.
	if len(manifest.Version) != 32 || manifest.Parts < 1 || manifest.Parts > 33 {
		return
	}
	for index := 0; index < manifest.Parts; index++ {
		_ = s.driver().Delete("opera", manifest.partName(name, index))
	}
}
