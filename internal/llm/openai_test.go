package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cirolini/explain/internal/prompt"
)

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

func TestNewOpenAIReportsModel(t *testing.T) {
	p, err := NewOpenAI(Options{APIKey: "sk-test", Model: "gpt-5.6-luna"})
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

// newStubbedProvider points an OpenAI adapter at a local test server, so the
// request/response round trip is exercised without reaching the real API.
func newStubbedProvider(t *testing.T, status int, body any) *OpenAI {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("unexpected path %q; explain should use Chat Completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Errorf("encoding stub response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	p, err := NewOpenAI(Options{APIKey: "sk-test", BaseURL: srv.URL, Model: "test-model"})
	if err != nil {
		t.Fatalf("NewOpenAI: %v", err)
	}
	return p
}

func TestCompleteReturnsExplanation(t *testing.T) {
	p := newStubbedProvider(t, http.StatusOK, map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"role": "assistant", "content": "  lists files  "}},
		},
	})

	got, err := p.Complete(context.Background(), prompt.Build("ls"))
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got != "lists files" {
		t.Errorf("Complete = %q, want %q", got, "lists files")
	}
}

// A model that spends its whole token budget on reasoning returns an empty
// string. That is a failure, not an explanation -- surface it rather than
// printing nothing and exiting 0.
func TestCompleteRejectsEmptyContent(t *testing.T) {
	p := newStubbedProvider(t, http.StatusOK, map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"role": "assistant", "content": "   "}},
		},
	})

	if _, err := p.Complete(context.Background(), prompt.Build("ls")); err == nil {
		t.Fatal("Complete succeeded on empty content, want error")
	}
}

func TestCompleteRejectsNoChoices(t *testing.T) {
	p := newStubbedProvider(t, http.StatusOK, map[string]any{"choices": []map[string]any{}})

	if _, err := p.Complete(context.Background(), prompt.Build("ls")); err == nil {
		t.Fatal("Complete succeeded with no choices, want error")
	}
}

// The failure mode that killed 1.x: a retired model id. It must surface as an
// error the caller can print, not a process exit from inside the library.
func TestCompleteReturnsAPIErrors(t *testing.T) {
	p := newStubbedProvider(t, http.StatusNotFound, map[string]any{
		"error": map[string]any{
			"message": "The model `text-davinci-003` does not exist",
			"code":    "model_not_found",
		},
	})

	_, err := p.Complete(context.Background(), prompt.Build("ls"))
	if err == nil {
		t.Fatal("Complete succeeded on a 404, want error")
	}
	if !strings.Contains(err.Error(), "openai-compatible") {
		t.Errorf("error does not name the provider: %v", err)
	}
}
