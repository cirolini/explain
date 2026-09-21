package llm

import (
	"context"

	"github.com/cirolini/explain/internal/prompt"
)

// Fake is an in-memory Provider for tests. It records the last request it was
// given so callers can assert on prompt construction without a network call.
type Fake struct {
	// Response is returned from Complete when Err is nil.
	Response string
	// Err, when set, is returned from Complete.
	Err error
	// ModelName is reported by Model.
	ModelName string

	// Last holds the most recent request passed to Complete.
	Last prompt.Request
	// Calls counts invocations of Complete.
	Calls int
}

// Name implements Provider.
func (f *Fake) Name() string { return "fake" }

// Model implements Provider.
func (f *Fake) Model() string { return f.ModelName }

// Complete implements Provider.
func (f *Fake) Complete(_ context.Context, req prompt.Request) (string, error) {
	f.Calls++
	f.Last = req
	if f.Err != nil {
		return "", f.Err
	}
	return f.Response, nil
}
