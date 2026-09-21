package llm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cirolini/explain/internal/prompt"
)

// req is the prompt used throughout these tests.
func req() prompt.Request { return prompt.Build("ls -lrth", prompt.EN, prompt.Context{}) }

func TestNewOpenAIRequiresCredentialsOrBaseURL(t *testing.T) {
	if _, err := NewOpenAI(Options{Model: "m"}); !errors.Is(err, ErrNoAPIKey) {
		t.Errorf("err = %v, want ErrNoAPIKey", err)
	}
}

// A local OpenAI-compatible server needs no key, so a base URL alone is enough.
func TestNewOpenAIAcceptsBaseURLWithoutKey(t *testing.T) {
	p, err := NewOpenAI(Options{BaseURL: "http://localhost:11434/v1", Model: "llama3"})
	if err != nil {
		t.Fatalf("NewOpenAI: %v", err)
	}
	if got := p.Name(); got != "openai-compatible" {
		t.Errorf("Name = %q, want %q", got, "openai-compatible")
	}
}

func TestNewOpenAIReportsNameAndModel(t *testing.T) {
	p, err := NewOpenAI(Options{APIKey: testKey, Model: "gpt-5.6-luna"})
	if err != nil {
		t.Fatalf("NewOpenAI: %v", err)
	}
	if got := p.Model(); got != "gpt-5.6-luna" {
		t.Errorf("Model = %q, want %q", got, "gpt-5.6-luna")
	}
	if got := p.Name(); got != "openai" {
		t.Errorf("Name = %q, want %q", got, "openai")
	}
}

// newOpenAIAgainst points an adapter at a test server.
func newOpenAIAgainst(t *testing.T, baseURL string) *OpenAI {
	t.Helper()
	p, err := NewOpenAI(Options{APIKey: testKey, BaseURL: baseURL, Model: "test-model"})
	if err != nil {
		t.Fatalf("NewOpenAI: %v", err)
	}
	return p
}

func TestOpenAICompleteReturnsTheWholeExplanation(t *testing.T) {
	srv := newSSEServer(t, openAIFrames("lists ", "files ", "by time")...)
	p := newOpenAIAgainst(t, srv.URL)

	got, err := p.Complete(context.Background(), req(), io.Discard)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if want := "lists files by time"; got != want {
		t.Errorf("Complete = %q, want %q", got, want)
	}
	if !strings.HasSuffix(srv.Path, "/chat/completions") {
		t.Errorf("path = %q; explain should use Chat Completions", srv.Path)
	}
}

// The point of streaming: fragments reach the writer as they arrive, so a
// terminal fills up instead of waiting for the whole response.
func TestOpenAICompleteStreamsToTheWriter(t *testing.T) {
	srv := newSSEServer(t, openAIFrames("lists ", "files")...)
	p := newOpenAIAgainst(t, srv.URL)

	var out bytes.Buffer
	if _, err := p.Complete(context.Background(), req(), &out); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if want := "lists files"; out.String() != want {
		t.Errorf("streamed %q, want %q", out.String(), want)
	}
}

// A model that spends its whole budget on reasoning emits no text. That is a
// failure, not an explanation -- surface it rather than exiting 0 silently.
func TestOpenAICompleteRejectsAnEmptyStream(t *testing.T) {
	srv := newSSEServer(t, openAIFrames()...)
	p := newOpenAIAgainst(t, srv.URL)

	if _, err := p.Complete(context.Background(), req(), io.Discard); err == nil {
		t.Fatal("Complete succeeded on an empty stream, want error")
	}
}

// The failure that killed 1.x: a retired model id. It must surface as an error
// the caller can print, not a process exit from inside the library.
func TestOpenAICompleteReturnsAPIErrors(t *testing.T) {
	srv := newErrorServer(t, http.StatusNotFound,
		`{"error":{"message":"The model `+"`text-davinci-003`"+` does not exist","code":"model_not_found"}}`)
	p := newOpenAIAgainst(t, srv.URL)

	_, err := p.Complete(context.Background(), req(), io.Discard)
	if err == nil {
		t.Fatal("Complete succeeded on a 404, want error")
	}
	if !strings.Contains(err.Error(), "openai-compatible") {
		t.Errorf("error does not name the provider: %v", err)
	}
}

func TestOpenAICompleteHonoursContextCancellation(t *testing.T) {
	srv := newSSEServer(t, openAIFrames("lists")...)
	p := newOpenAIAgainst(t, srv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := p.Complete(ctx, req(), io.Discard); err == nil {
		t.Fatal("Complete succeeded with a cancelled context, want error")
	}
}
