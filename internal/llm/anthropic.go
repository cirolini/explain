package llm

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/cirolini/explain/internal/prompt"
)

// Anthropic talks to the Claude API through the official SDK, rather than
// through the OpenAI-compatibility endpoint, which Anthropic documents as a
// testing aid rather than a production path.
type Anthropic struct {
	client anthropic.Client
	model  string
}

// NewAnthropic builds a Claude adapter.
func NewAnthropic(opts Options) (*Anthropic, error) {
	if opts.APIKey == "" {
		return nil, ErrNoAPIKey
	}

	reqOpts := []option.RequestOption{option.WithAPIKey(opts.APIKey)}
	// A base URL lets the adapter reach a gateway or proxy in front of the
	// Claude API, and lets the tests point it at a local server.
	if opts.BaseURL != "" {
		reqOpts = append(reqOpts, option.WithBaseURL(opts.BaseURL))
	}

	return &Anthropic{
		client: anthropic.NewClient(reqOpts...),
		model:  opts.Model,
	}, nil
}

// Name implements Provider.
func (a *Anthropic) Name() string { return "anthropic" }

// Model implements Provider.
func (a *Anthropic) Model() string { return a.model }

// Complete implements Provider.
//
// Effort is set to low deliberately. Describing a shell command is not a
// reasoning-heavy task, and on models where thinking is on by default a higher
// effort means the user watches a blank terminal while the model deliberates.
// The risk verdict does not depend on this: from v2 it comes from rules that
// never consult a model.
func (a *Anthropic) Complete(ctx context.Context, req prompt.Request, w io.Writer) (string, error) {
	stream := a.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: maxOutputTokens,
		System:    []anthropic.TextBlockParam{{Text: req.System}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(req.User)),
		},
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortLow},
	})
	defer stream.Close() //nolint:errcheck // the stream error is reported via stream.Err below

	var b strings.Builder
	for stream.Next() {
		delta, ok := textDelta(stream.Current())
		if !ok {
			continue
		}
		b.WriteString(delta)
		if _, err := io.WriteString(w, delta); err != nil {
			return "", fmt.Errorf("anthropic: writing output: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		return "", fmt.Errorf("anthropic: %w", err)
	}

	text := strings.TrimSpace(b.String())
	if text == "" {
		return "", fmt.Errorf("anthropic: model %q returned an empty explanation", a.model)
	}
	return text, nil
}

// textDelta pulls visible text out of a stream event. Thinking deltas are
// skipped: they are not the explanation, and on current models they arrive
// empty anyway.
func textDelta(event anthropic.MessageStreamEventUnion) (string, bool) {
	d, ok := event.AsAny().(anthropic.ContentBlockDeltaEvent)
	if !ok {
		return "", false
	}
	if d.Delta.Type != "text_delta" || d.Delta.Text == "" {
		return "", false
	}
	return d.Delta.Text, true
}
