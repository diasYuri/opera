package credentials

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// FileStore is an explicitly selected, unencrypted fallback. It never replaces
// the keyring automatically. One atomic file per reference avoids lost updates.
type FileStore struct {
	Directory string
}

func (s FileStore) path(name string) (string, error) {
	if err := validate(name); err != nil {
		return "", err
	}
	return filepath.Join(s.Directory, fmt.Sprintf("%x.secret", sha256.Sum256([]byte(name)))), nil
}

func (s FileStore) prepare(create bool) error {
	if !filepath.IsAbs(s.Directory) {
		return fmt.Errorf("credential directory must be absolute")
	}
	if create {
		parent := filepath.Dir(s.Directory)
		if err := os.MkdirAll(filepath.Dir(parent), 0700); err != nil {
			return fmt.Errorf("create credential parent: %w", err)
		}
		if err := os.Mkdir(parent, 0700); err == nil {
			if err := restrict(parent, true); err != nil {
				return err
			}
		} else if !os.IsExist(err) {
			return fmt.Errorf("create credential parent: %w", err)
		}
		if err := checkPrivate(parent, true); err != nil {
			return err
		}
		if err := os.Mkdir(s.Directory, 0700); err == nil {
			if err := restrict(s.Directory, true); err != nil {
				return err
			}
		} else if !os.IsExist(err) {
			return fmt.Errorf("create credential directory: %w", err)
		}
	}
	if err := checkPrivate(s.Directory, true); err != nil {
		return err
	}
	return checkPrivate(filepath.Dir(s.Directory), true)
}

func (s FileStore) Set(name, secret string) error {
	path, err := s.path(name)
	if err != nil {
		return err
	}
	if err := validateSecret(secret); err != nil {
		return err
	}
	if err := s.prepare(true); err != nil {
		return err
	}
	if err := checkPrivate(path, false); err != nil && !os.IsNotExist(err) {
		return err
	}
	file, err := os.CreateTemp(s.Directory, ".credential-*")
	if err != nil {
		return fmt.Errorf("create credential: %w", err)
	}
	defer func() { _ = file.Close(); _ = os.Remove(file.Name()) }()
	if err := restrict(file.Name(), false); err != nil {
		return err
	}
	if _, err := io.WriteString(file, secret); err != nil {
		return fmt.Errorf("write credential: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync credential: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close credential: %w", err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("commit credential: %w", err)
	}
	return nil
}

func (s FileStore) Get(name string) (string, error) {
	path, err := s.path(name)
	if err != nil {
		return "", err
	}
	if err := s.prepare(false); err != nil {
		return "", missing(err)
	}
	if err := checkPrivate(path, false); err != nil {
		return "", missing(err)
	}
	file, err := os.Open(path)
	if err != nil {
		return "", missing(err)
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, 65537))
	if err != nil {
		return "", fmt.Errorf("read credential: %w", err)
	}
	if err := validateSecret(string(data)); err != nil {
		return "", err
	}
	return string(data), nil
}

func (s FileStore) Delete(name string) error {
	path, err := s.path(name)
	if err != nil {
		return err
	}
	if err := s.prepare(false); err != nil {
		return missing(err)
	}
	if err := checkPrivate(path, false); err != nil {
		return missing(err)
	}
	return missing(os.Remove(path))
}

func missing(err error) error {
	if os.IsNotExist(err) {
		return ErrNotFound
	}
	return err
}
