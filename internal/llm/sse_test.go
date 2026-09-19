package llm

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testKey stands in for a credential in tests. It deliberately does not look
// like a real key prefix, so secret scanners have nothing to flag.
const testKey = "unit-test-key"

// sseServer serves a fixed sequence of Server-Sent Events, so the streaming
// adapters can be exercised without reaching a real API. It records the last
// request path.
type sseServer struct {
	*httptest.Server
	Path string
}

// newSSEServer returns a server that replies to any POST with the given SSE
// frames, each already formatted as it appears on the wire.
func newSSEServer(t *testing.T, frames ...string) *sseServer {
	t.Helper()

	s := &sseServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Path = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, f := range frames {
			if _, err := fmt.Fprint(w, f); err != nil {
				t.Errorf("writing SSE frame: %v", err)
				return
			}
			w.(http.Flusher).Flush()
		}
	}))
	t.Cleanup(s.Close)
	return s
}

// newErrorServer returns a server that fails every request with status and body.
func newErrorServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := fmt.Fprint(w, body); err != nil {
			t.Errorf("writing error body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// openAIFrames formats text fragments as OpenAI chat-completion stream chunks.
func openAIFrames(deltas ...string) []string {
	frames := make([]string, 0, len(deltas)+1)
	for _, d := range deltas {
		frames = append(frames, fmt.Sprintf(
			"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":%s}}]}\n\n", quote(d)))
	}
	return append(frames, "data: [DONE]\n\n")
}

// anthropicFrames formats text fragments as Claude message stream events.
func anthropicFrames(deltas ...string) []string {
	frames := []string{
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\"," +
			"\"type\":\"message\",\"role\":\"assistant\",\"model\":\"test\",\"content\":[]," +
			"\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n",
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0," +
			"\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n",
	}
	for _, d := range deltas {
		frames = append(frames, fmt.Sprintf(
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,"+
				"\"delta\":{\"type\":\"text_delta\",\"text\":%s}}\n\n", quote(d)))
	}
	return append(frames,
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
}

// quote renders s as a JSON string literal.
func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s) + `"`
}
