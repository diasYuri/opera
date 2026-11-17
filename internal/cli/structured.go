package cli

import (
	"errors"
	"io"
	"os"

	"github.com/diasYuri/y/pkg/ai"
)

func readResponseFormat(kind, path string) (*ai.ResponseFormat, error) {
	if kind == "" && path == "" {
		return nil, nil
	}
	format := &ai.ResponseFormat{Type: ai.ResponseFormatKind(kind)}
	if path != "" {
		if kind != "" && kind != "json_schema" {
			return nil, errors.New("response schema conflicts with response format")
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer func() { _ = file.Close() }()
		raw, err := io.ReadAll(io.LimitReader(file, 1<<20+1))
		if err != nil || len(raw) > 1<<20 {
			return nil, errors.New("response schema exceeds 1 MiB")
		}
		format.Type = ai.ResponseFormatJSONSchema
		format.Name = "opera_result"
		format.Strict = true
		format.Schema = raw
	}
	if err := ai.ValidateResponseFormat(*format); err != nil {
		return nil, err
	}
	return format, nil
}
