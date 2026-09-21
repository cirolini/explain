package llm

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/cirolini/explain/internal/prompt"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

// OpenAI talks to the Chat Completions API. It also serves any
// OpenAI-compatible endpoint -- Ollama, vLLM, LM Studio -- via BaseURL, which
// is why explain uses Chat Completions rather than the newer Responses API:
// local servers implement the former.
type OpenAI struct {
	client openai.Client
	model  string
	name   string
}

// NewOpenAI builds an OpenAI adapter. A BaseURL pointing at a local server
// makes the API key optional, since such servers usually accept any value.
func NewOpenAI(opts Options) (*OpenAI, error) {
	if opts.APIKey == "" && opts.BaseURL == "" {
		return nil, ErrNoAPIKey
	}

	reqOpts := []option.RequestOption{}
	if opts.APIKey != "" {
		reqOpts = append(reqOpts, option.WithAPIKey(opts.APIKey))
	}

	name := "openai"
	if opts.BaseURL != "" {
		reqOpts = append(reqOpts, option.WithBaseURL(opts.BaseURL))
		name = "openai-compatible"
	}

	return &OpenAI{
		client: openai.NewClient(reqOpts...),
		model:  opts.Model,
		name:   name,
	}, nil
}

// Name implements Provider.
func (o *OpenAI) Name() string { return o.name }

// Model implements Provider.
func (o *OpenAI) Model() string { return o.model }

// Complete implements Provider.
//
// Temperature is deliberately left unset: reasoning-capable models reject any
// value other than the default, and explain has no need to vary it.
func (o *OpenAI) Complete(ctx context.Context, req prompt.Request, w io.Writer) (string, error) {
	stream := o.client.Chat.Completions.NewStreaming(ctx, openai.ChatCompletionNewParams{
		Model: shared.ChatModel(o.model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(req.System),
			openai.UserMessage(req.User),
		},
		MaxCompletionTokens: openai.Int(maxOutputTokens),
	})
	defer stream.Close() //nolint:errcheck // the stream error is reported via stream.Err below

	var b strings.Builder
	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta.Content
		if delta == "" {
			continue
		}
		b.WriteString(delta)
		if _, err := io.WriteString(w, delta); err != nil {
			return "", fmt.Errorf("%s: writing output: %w", o.name, err)
		}
	}
	if err := stream.Err(); err != nil {
		return "", fmt.Errorf("%s: %w", o.name, err)
	}

	text := strings.TrimSpace(b.String())
	if text == "" {
		return "", fmt.Errorf("%s: model %q %w", o.name, o.model, ErrEmptyExplanation)
	}
	return text, nil
}
