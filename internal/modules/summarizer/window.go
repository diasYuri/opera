package summarizer

import (
	"github.com/diasYuri/y/pkg/ai"
	"github.com/diasYuri/y/pkg/providers"
)

const MemoryTokens = 5000

// Window measures the actual outgoing input, including instructions and tools.
// InputTokens is a local estimate, independent of provider billing.
type Window struct {
	InputTokens, ContextTokens, InputLimit, OutputTokens int64
	ThresholdTokens                                      int64
}

func Measure(request providers.StreamRequest, model ai.Model, configured int64) Window {
	output := request.Options.MaxTokens
	if model.MaxTokens > 0 {
		output = min(output, model.MaxTokens)
	}
	window := model.ContextWindow
	limit := configured
	if window > 0 {
		// Keep an output allowance even for models with small windows.
		output = min(output, max(1, window/4))
		limit = min(limit, window-output)
	} else {
		window = configured
	}
	return Window{
		InputTokens:   providers.EstimateTokens(request.Context),
		ContextTokens: window, InputLimit: limit, OutputTokens: output,
		ThresholdTokens: (3*min(configured, window) + 4) / 5,
	}
}

func (w Window) ShouldCompact() bool {
	return w.ThresholdTokens > 0 && (w.InputTokens >= w.ThresholdTokens || w.InputTokens > w.InputLimit)
}
