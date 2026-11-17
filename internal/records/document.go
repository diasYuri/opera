// Package records contains I/O-independent durable record values shared by
// application contracts and persistence adapters.
package records

import "encoding/json"

type Document struct {
	Key     string          `json:"key"`
	Version uint64          `json:"version"`
	Data    json.RawMessage `json:"data"`
}
