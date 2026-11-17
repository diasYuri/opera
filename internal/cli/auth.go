package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/diasYuri/opera/internal/adapters/credentials"
	"github.com/diasYuri/opera/internal/modules/configuration"
)

func (a App) auth(args []string, cfg config.Resolved, jsonl bool) int {
	if len(args) != 2 || args[0] != "set" && args[0] != "status" && args[0] != "delete" {
		return a.fail(ExitUsage, errors.New("usage: opera auth <set|status|delete> <reference> [--store keyring|file]; set reads the secret from non-TTY stdin"), jsonl)
	}
	store, err := a.OpenStore(cfg.String("credentials.store"), cfg.Paths.Credentials)
	if err != nil {
		return a.fail(ExitFailure, err, jsonl)
	}
	status := "stored"
	switch args[0] {
	case "set":
		if a.IsTTY {
			return a.fail(ExitUsage, errors.New("auth set requires piped stdin; use auth login for supported OAuth providers"), jsonl)
		}
		data, readErr := io.ReadAll(io.LimitReader(a.In, 65539))
		if readErr != nil || len(data) > 65538 {
			return a.fail(ExitUsage, errors.New("cannot read credential (limit: 65536 bytes plus line ending)"), jsonl)
		}
		// Strip exactly one conventional line ending, never meaningful spaces.
		secret := string(data)
		if strings.HasSuffix(secret, "\r\n") {
			secret = strings.TrimSuffix(secret, "\r\n")
		} else {
			secret = strings.TrimSuffix(secret, "\n")
		}
		err = store.Set(args[1], secret)
	case "status":
		_, err = store.Get(args[1])
		if errors.Is(err, credentials.ErrNotFound) {
			err = nil
			status = "missing"
		}
	case "delete":
		err = store.Delete(args[1])
		status = "deleted"
	}
	if err != nil {
		return a.fail(ExitFailure, err, jsonl)
	}
	if jsonl {
		return a.emit("auth.status", map[string]string{"status": status, "store": cfg.String("credentials.store")})
	}
	return a.writeText(fmt.Sprintf("Credential %s (%s store).\n", status, cfg.String("credentials.store")))
}
