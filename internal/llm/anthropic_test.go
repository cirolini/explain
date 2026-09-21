package llm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestNewAnthropicRequiresAnAPIKey(t *testing.T) {
	if _, err := NewAnthropic(Options{Model: "claude-opus-5"}); !errors.Is(err, ErrNoAPIKey) {
		t.Errorf("err = %v, want ErrNoAPIKey", err)
	}
}

func TestNewAnthropicReportsNameAndModel(t *testing.T) {
	p, err := NewAnthropic(Options{APIKey: testKey, Model: "claude-opus-5"})
	if err != nil {
		t.Fatalf("NewAnthropic: %v", err)
	}
	if got := p.Name(); got != "anthropic" {
		t.Errorf("Name = %q, want %q", got, "anthropic")
	}
	if got := p.Model(); got != "claude-opus-5" {
		t.Errorf("Model = %q, want %q", got, "claude-opus-5")
	}
}

// newAnthropicAgainst points an adapter at a test server.
func newAnthropicAgainst(t *testing.T, baseURL string) *Anthropic {
	t.Helper()
	p, err := NewAnthropic(Options{APIKey: testKey, BaseURL: baseURL, Model: "test-model"})
	if err != nil {
		t.Fatalf("NewAnthropic: %v", err)
	}
	return p
}

func TestAnthropicCompleteReturnsTheWholeExplanation(t *testing.T) {
	srv := newSSEServer(t, anthropicFrames("lists ", "files ", "by time")...)
	p := newAnthropicAgainst(t, srv.URL)

	got, err := p.Complete(context.Background(), req(), io.Discard)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if want := "lists files by time"; got != want {
		t.Errorf("Complete = %q, want %q", got, want)
	}
	if !strings.HasSuffix(srv.Path, "/messages") {
		t.Errorf("path = %q, want the Messages API", srv.Path)
	}
}

func TestAnthropicCompleteStreamsToTheWriter(t *testing.T) {
	srv := newSSEServer(t, anthropicFrames("lists ", "files")...)
	p := newAnthropicAgainst(t, srv.URL)

	var out bytes.Buffer
	if _, err := p.Complete(context.Background(), req(), &out); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if want := "lists files"; out.String() != want {
		t.Errorf("streamed %q, want %q", out.String(), want)
	}
}

func TestAnthropicCompleteRejectsAnEmptyStream(t *testing.T) {
	srv := newSSEServer(t, anthropicFrames()...)
	p := newAnthropicAgainst(t, srv.URL)

	_, err := p.Complete(context.Background(), req(), io.Discard)
	if !errors.Is(err, ErrEmptyExplanation) {
		t.Fatalf("err = %v, want ErrEmptyExplanation", err)
	}
}

func TestAnthropicCompleteReturnsAPIErrors(t *testing.T) {
	srv := newErrorServer(t, http.StatusNotFound,
		`{"type":"error","error":{"type":"not_found_error","message":"model not found"}}`)
	p := newAnthropicAgainst(t, srv.URL)

	_, err := p.Complete(context.Background(), req(), io.Discard)
	if err == nil {
		t.Fatal("Complete succeeded on a 404, want error")
	}
	if !strings.Contains(err.Error(), "anthropic") {
		t.Errorf("error does not name the provider: %v", err)
	}
}
