// Package output implements the versioned headless output contract.
package output

import (
	"encoding/json"
	"io"
)

const SchemaVersion = 1

type Event struct {
	SchemaVersion int    `json:"schema_version"`
	Sequence      int    `json:"sequence"`
	Type          string `json:"type"`
	Data          any    `json:"data"`
}

// WriteResult emits one complete event. Execution events will add correlation
// IDs when the execution service is introduced in F1.
func WriteResult(writer io.Writer, kind string, data any) error {
	return json.NewEncoder(writer).Encode(Event{SchemaVersion, 1, kind, data})
}
