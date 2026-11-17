package harness

import (
	"context"
	"github.com/diasYuri/y/pkg/providers"
	"time"
)

type summarizer struct {
	provider   providers.Provider
	credential func(context.Context, string) (string, error)
	timeout    string
}

func (s summarizer) Stream(ctx context.Context, request providers.StreamRequest) (providers.EventStream, error) {
	if s.credential != nil {
		key, err := s.credential(ctx, s.provider.ID())
		if err != nil {
			return nil, err
		}
		request.Options.APIKey = key
	}
	request.Options.Timeout, _ = time.ParseDuration(s.timeout)
	request.Options.MaxTokens = 4096
	return s.provider.Stream(ctx, request)
}
