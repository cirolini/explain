package report

import (
	"bytes"
	"io"
	"strings"

	"github.com/cirolini/explain/internal/risk"
)

// severityPrefix is the marker the model is asked to open its answer with.
const severityPrefix = "SEVERITY:"

// SeverityFilter passes a model's streamed answer through to an underlying
// writer while removing the leading "SEVERITY: X" line and recording what it
// said.
//
// It exists so the explanation can stream: the severity arrives first, in the
// middle of a stream that is already being printed, and the user should not
// see explain's own machinery in the output. Everything up to the first
// newline is held back; if it turns out not to be a severity line, it is
// written out unchanged.
type SeverityFilter struct {
	w io.Writer

	head bytes.Buffer
	done bool
	// trimming holds while the blank lines that follow a stripped marker are
	// still being discarded. They can arrive in a later chunk than the marker
	// itself, so this cannot be done in one pass over the first write.
	trimming bool
	severity risk.Severity
	found    bool
}

// NewSeverityFilter wraps w.
func NewSeverityFilter(w io.Writer) *SeverityFilter { return &SeverityFilter{w: w} }

// Severity reports the severity the model gave, and whether it gave one.
func (f *SeverityFilter) Severity() (risk.Severity, bool) { return f.severity, f.found }

// Write implements io.Writer.
func (f *SeverityFilter) Write(p []byte) (int, error) {
	if f.done {
		return len(p), f.writeBody(p)
	}

	n := len(p)
	f.head.Write(p)

	line, rest, complete := bytes.Cut(f.head.Bytes(), []byte("\n"))
	if !complete {
		// Still inside the first line; hold it back until we know what it is.
		return n, nil
	}
	f.done = true

	if sev, ok := parseSeverityLine(string(line)); ok {
		f.severity, f.found = sev, true
		// Drop the marker line and the blank lines separating it from the
		// prose, however many chunks they turn up in.
		f.trimming = true
	} else {
		rest = f.head.Bytes()
	}

	return n, f.writeBody(rest)
}

// writeBody forwards explanation text, discarding the blank lines that
// followed a stripped marker.
func (f *SeverityFilter) writeBody(p []byte) error {
	if f.trimming {
		p = bytes.TrimLeft(p, "\n\r\t ")
		if len(p) == 0 {
			return nil
		}
		f.trimming = false
	}
	if len(p) == 0 {
		return nil
	}
	_, err := f.w.Write(p)
	return err
}

// Flush writes anything still held back, for an answer that never contained a
// newline at all.
func (f *SeverityFilter) Flush() error {
	if f.done {
		return nil
	}
	f.done = true

	if sev, ok := parseSeverityLine(f.head.String()); ok {
		f.severity, f.found = sev, true
		return nil
	}
	_, err := f.w.Write(f.head.Bytes())
	return err
}

// parseSeverityLine reads a "SEVERITY: HIGH" line, tolerating the decoration a
// model may add around it, such as bold markers or a trailing note.
func parseSeverityLine(line string) (risk.Severity, bool) {
	clean := strings.TrimSpace(strings.ReplaceAll(line, "*", ""))
	if !strings.HasPrefix(strings.ToUpper(clean), severityPrefix) {
		return risk.Low, false
	}

	value := strings.TrimSpace(clean[len(severityPrefix):])
	// Keep only the first word, so "HIGH -- destroys data" still reads.
	if i := strings.IndexAny(value, " \t-–—,."); i > 0 {
		value = value[:i]
	}

	sev, err := risk.ParseSeverity(value)
	if err != nil {
		return risk.Low, false
	}
	return sev, true
}

// StripSeverityLine removes a leading severity marker from a complete answer,
// for the buffered path where nothing was streamed.
func StripSeverityLine(text string) (risk.Severity, bool, string) {
	line, rest, found := strings.Cut(text, "\n")
	if !found {
		if sev, ok := parseSeverityLine(text); ok {
			return sev, true, ""
		}
		return risk.Low, false, text
	}

	sev, ok := parseSeverityLine(line)
	if !ok {
		return risk.Low, false, text
	}
	return sev, true, strings.TrimLeft(rest, "\n")
}
