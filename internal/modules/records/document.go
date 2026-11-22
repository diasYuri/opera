// Package records contains I/O-independent durable record values shared by
// application contracts and persistence adapters.
package records

import (
	"encoding/json"
	"errors"
)

var ErrConflict = errors.New("opera record version conflict")

type Document struct {
	Key     string          `json:"key"`
	Version uint64          `json:"version"`
	Data    json.RawMessage `json:"data"`
}
