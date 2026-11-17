package execution

import (
	"context"
	"encoding/json"

	"github.com/diasYuri/opera/internal/modules/records"
	"github.com/diasYuri/y/pkg/agent"
	"github.com/diasYuri/y/pkg/tools"
)

// ExecutionStore combines the public Y durability contracts with Opera's
// checkpoint metadata and operation inspection needs.
type ExecutionStore interface {
	agent.StateStore
	agent.EventStore
	agent.TranscriptStore
	agent.OperationStore
	Operations(context.Context, string) ([]agent.OperationRecord, error)
	GetMetadata(context.Context, string) (json.RawMessage, error)
	PutMetadata(context.Context, string, json.RawMessage) error
	CompleteOperation(context.Context, string, tools.ToolResponse, error) error
}

// Records exposes the small document operations consumed by application use
// cases. Persistence ownership and schema remain in the composition adapter.
type Records interface {
	Get(context.Context, string, string) (records.Document, error)
	List(context.Context, string) ([]records.Document, error)
	Put(context.Context, string, string, any) error
}
