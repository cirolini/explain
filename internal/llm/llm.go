// Package llm defines the provider abstraction explain talks to, plus the
// adapters that implement it.
package llm

import (
	"context"
	"errors"
	"io"

	"github.com/cirolini/explain/internal/prompt"
)

// ErrNoAPIKey reports that a provider requiring credentials was given none.
var ErrNoAPIKey = errors.New("provider requires an API key")

// ErrEmptyExplanation reports that the model answered with no visible text --
// typically a reasoning model that spent its whole budget before writing
// anything. It is a sentinel so callers that measure models can count it as
// "no answer" rather than treating it as a failure of the run.
var ErrEmptyExplanation = errors.New("returned an empty explanation")

// Provider turns a prompt into an explanation.
type Provider interface {
	// Name identifies the provider, for error messages and --json output.
	Name() string
	// Model reports the model this provider was configured with.
	Model() string
	// Complete returns the whole explanation, writing it to w in fragments as
	// they arrive so a terminal can print it without waiting for the end. Pass
	// io.Discard when only the return value is wanted.
	//
	// Because output is written as it arrives, an error can be returned after
	// w has already received part of an explanation. Callers that must not show
	// partial output should pass io.Discard and print the returned string.
	Complete(ctx context.Context, req prompt.Request, w io.Writer) (string, error)
}

// Options configures an adapter. Not every field applies to every provider.
type Options struct {
	APIKey  string
	BaseURL string
	Model   string
}

// maxOutputTokens bounds the explanation. explain 1.x set MaxTokens to 4000
// against a 4097-token context window, leaving almost no room for the prompt;
// this bounds only the response. It is set well above what an explanation
// needs because current models spend part of the budget on internal reasoning
// before emitting visible text -- too low and the response comes back empty
// rather than truncated.
const maxOutputTokens = 2000
