// Package llm defines the provider abstraction explain talks to, plus the
// adapters that implement it.
//
// Phase 0 ships one adapter (OpenAI, which also covers OpenAI-compatible
// servers via a custom base URL). The interface exists now so that adding
// Anthropic and streaming later does not disturb the call sites.
package llm

import (
	"context"
	"errors"

	"github.com/cirolini/explain/internal/prompt"
)

// ErrNoAPIKey reports that a provider requiring credentials was given none.
var ErrNoAPIKey = errors.New("provider requires an API key")

// Provider turns a prompt into an explanation.
type Provider interface {
	// Name identifies the provider, for error messages and --json output.
	Name() string
	// Model reports the model this provider was configured with.
	Model() string
	// Complete returns the model's response, or an error. It never exits the
	// process and never writes to stdout.
	Complete(ctx context.Context, req prompt.Request) (string, error)
}

// Options configures an adapter. Not every field applies to every provider.
type Options struct {
	APIKey  string
	BaseURL string
	Model   string
}
