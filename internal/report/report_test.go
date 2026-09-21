package report

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/cirolini/explain/internal/risk"
)

func verdict(t *testing.T, command string) risk.Verdict {
	t.Helper()
	return risk.MustDefault().Evaluate(command)
}

// A model may argue a command is worse than the rules found. It may never
// argue that it is safer.
func TestModelSeverityRaisesButNeverLowers(t *testing.T) {
	high := New("rm -rf /", verdict(t, "rm -rf /"))
	if high.Severity != risk.High {
		t.Fatalf("setup: severity = %s, want HIGH", high.Severity)
	}

	for _, model := range []risk.Severity{risk.Low, risk.Medium, risk.High} {
		got := high.WithModelSeverity(model)
		if got.Severity != risk.High {
			t.Errorf("model said %s and the verdict became %s", model, got.Severity)
		}
		if got.RuleSeverity != risk.High {
			t.Errorf("rule severity was overwritten: %s", got.RuleSeverity)
		}
	}

	low := New("ls", verdict(t, "ls"))
	if got := low.WithModelSeverity(risk.High); got.Severity != risk.High {
		t.Errorf("model said HIGH but the verdict stayed %s", got.Severity)
	}
}

func TestVerdictLineLeadsWithTheRuleThatSetIt(t *testing.T) {
	r := New("rm -rf /", verdict(t, "rm -rf /"))

	line := r.Verdict()
	if !strings.HasPrefix(line, "HIGH — ") {
		t.Errorf("verdict = %q, want it to start with the severity", line)
	}
	if strings.Contains(line, "\n") {
		t.Errorf("verdict spans multiple lines: %q", line)
	}
}

func TestVerdictLineForACleanCommand(t *testing.T) {
	r := New("ls -lrth", verdict(t, "ls -lrth"))

	if !strings.HasPrefix(r.Verdict(), "LOW") {
		t.Errorf("verdict = %q, want LOW", r.Verdict())
	}
}

func TestWriteVerdictListsEveryFinding(t *testing.T) {
	r := New("sudo rm -rf /var", verdict(t, "sudo rm -rf /var"))
	if len(r.Findings) < 2 {
		t.Skipf("need at least two findings, got %d", len(r.Findings))
	}

	var out bytes.Buffer
	if err := r.WriteVerdict(&out); err != nil {
		t.Fatalf("WriteVerdict: %v", err)
	}
	for _, f := range r.Findings {
		if !strings.Contains(out.String(), collapse(f.Reason)) {
			t.Errorf("finding %q missing from output:\n%s", f.RuleID, out.String())
		}
	}
}

func TestEscalationOnlyPrintsWhenTheModelRaised(t *testing.T) {
	base := New("ls", verdict(t, "ls"))
	base.Provider = "openai"

	var raised bytes.Buffer
	if err := base.WithModelSeverity(risk.High).WriteEscalation(&raised); err != nil {
		t.Fatalf("WriteEscalation: %v", err)
	}
	if !strings.Contains(raised.String(), "Raised to HIGH") {
		t.Errorf("no escalation note:\n%s", raised.String())
	}

	var quiet bytes.Buffer
	if err := base.WithModelSeverity(risk.Low).WriteEscalation(&quiet); err != nil {
		t.Fatalf("WriteEscalation: %v", err)
	}
	if quiet.String() != "" {
		t.Errorf("escalation printed when nothing was raised:\n%s", quiet.String())
	}
}

func TestWriteJSONIsValidAndCarriesBothSeverities(t *testing.T) {
	r := New("rm -rf /", verdict(t, "rm -rf /")).WithModelSeverity(risk.Medium)
	r.Explanation = "Deletes everything."
	r.Provider, r.Model = "openai", "gpt-5.6-luna"

	var out bytes.Buffer
	if err := r.WriteJSON(&out); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out.String())
	}
	for field, want := range map[string]any{
		"severity":       "HIGH",
		"rule_severity":  "HIGH",
		"model_severity": "MEDIUM",
		"command":        "rm -rf /",
		"parsed":         true,
	} {
		if got[field] != want {
			t.Errorf("%s = %v, want %v", field, got[field], want)
		}
	}
}

func TestJSONOmitsModelSeverityWhenTheModelGaveNone(t *testing.T) {
	var out bytes.Buffer
	if err := New("ls", verdict(t, "ls")).WriteJSON(&out); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	if strings.Contains(out.String(), "model_severity") {
		t.Errorf("model_severity present with no model rating:\n%s", out.String())
	}
}

// The filter must remove explain's own marker from what the user sees, while
// still letting the explanation stream through in fragments.
func TestSeverityFilterStripsTheMarkerLine(t *testing.T) {
	var out bytes.Buffer
	f := NewSeverityFilter(&out)

	for _, chunk := range []string{"SEVER", "ITY: HIGH\n", "\nDeletes ", "everything."} {
		if _, err := io.WriteString(f, chunk); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	if err := f.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	if got := out.String(); got != "Deletes everything." {
		t.Errorf("output = %q, want the explanation with no marker", got)
	}
	sev, ok := f.Severity()
	if !ok || sev != risk.High {
		t.Errorf("Severity = (%s, %v), want (HIGH, true)", sev, ok)
	}
}

func TestSeverityFilterPassesThroughWhenThereIsNoMarker(t *testing.T) {
	var out bytes.Buffer
	f := NewSeverityFilter(&out)

	if _, err := io.WriteString(f, "Deletes everything.\nNo undo."); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := f.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	if got := out.String(); got != "Deletes everything.\nNo undo." {
		t.Errorf("output = %q, want the text unchanged", got)
	}
	if _, ok := f.Severity(); ok {
		t.Error("Severity reported a value where there was no marker")
	}
}

func TestSeverityFilterToleratesDecoration(t *testing.T) {
	for _, line := range []string{
		"SEVERITY: HIGH", "**SEVERITY: HIGH**", "severity: high",
		"SEVERITY:  HIGH -- destroys data", "SEVERITY: High.",
	} {
		t.Run(line, func(t *testing.T) {
			var out bytes.Buffer
			f := NewSeverityFilter(&out)
			if _, err := io.WriteString(f, line+"\n\nprose"); err != nil {
				t.Fatalf("Write: %v", err)
			}
			if err := f.Flush(); err != nil {
				t.Fatalf("Flush: %v", err)
			}

			sev, ok := f.Severity()
			if !ok || sev != risk.High {
				t.Errorf("Severity = (%s, %v), want (HIGH, true)", sev, ok)
			}
			if out.String() != "prose" {
				t.Errorf("output = %q, want %q", out.String(), "prose")
			}
		})
	}
}

func TestStripSeverityLine(t *testing.T) {
	sev, ok, rest := StripSeverityLine("SEVERITY: MEDIUM\n\nChanges permissions.")
	if !ok || sev != risk.Medium {
		t.Errorf("severity = (%s, %v), want (MEDIUM, true)", sev, ok)
	}
	if rest != "Changes permissions." {
		t.Errorf("rest = %q, want the explanation alone", rest)
	}

	if _, ok, rest := StripSeverityLine("Just prose.\nMore."); ok || rest != "Just prose.\nMore." {
		t.Errorf("StripSeverityLine altered text with no marker: %q", rest)
	}
}
