package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/cirolini/explain/internal/prompt"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

// maxCompletionTokens bounds the response. explain 1.x set MaxTokens to 4000
// against a 4097-token context window, leaving almost no room for the prompt;
// the ceiling here is for the completion only. Current models spend part of
// this budget on internal reasoning before emitting any visible text, so it is
// set well above what an explanation needs -- too low and the response comes
// back empty rather than truncated.
const maxCompletionTokens = 2000

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
func (o *OpenAI) Complete(ctx context.Context, req prompt.Request) (string, error) {
	resp, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: shared.ChatModel(o.model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(req.System),
			openai.UserMessage(req.User),
		},
		MaxCompletionTokens: openai.Int(maxCompletionTokens),
	})
	if err != nil {
		return "", fmt.Errorf("%s: %w", o.name, err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("%s: model %q returned no choices", o.name, o.model)
	}

	text := strings.TrimSpace(resp.Choices[0].Message.Content)
	if text == "" {
		return "", fmt.Errorf("%s: model %q returned an empty explanation", o.name, o.model)
	}
	return text, nil
}
