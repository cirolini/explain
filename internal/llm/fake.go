package llm

import (
	"context"
	"io"

	"github.com/cirolini/explain/internal/prompt"
)

// Every adapter must satisfy Provider. Without these the compiler is happy to
// let an adapter drift out of the interface until a call site notices.
var (
	_ Provider = (*OpenAI)(nil)
	_ Provider = (*Anthropic)(nil)
	_ Provider = (*Fake)(nil)
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
func (f *Fake) Complete(_ context.Context, req prompt.Request, w io.Writer) (string, error) {
	f.Calls++
	f.Last = req
	if f.Err != nil {
		return "", f.Err
	}
	if _, err := io.WriteString(w, f.Response); err != nil {
		return "", err
	}
	return f.Response, nil
}
