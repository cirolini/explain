// Package report renders a verdict and explanation for a terminal or for
// another program.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/cirolini/explain/internal/risk"
)

// Report is everything explain concluded about one command.
type Report struct {
	// Command is the command that was assessed, never run.
	Command string `json:"command"`
	// Severity is the final verdict: the rules' severity, raised by the model
	// if the model rated it higher, never lowered.
	Severity risk.Severity `json:"severity"`
	// RuleSeverity is what the deterministic rules alone concluded.
	RuleSeverity risk.Severity `json:"rule_severity"`
	// ModelSeverity is what the model rated it, when it gave a rating.
	ModelSeverity *risk.Severity `json:"model_severity,omitempty"`
	// Findings are the rules that fired, most severe first.
	Findings []risk.Finding `json:"findings"`
	// Parsed reports whether the shell parser understood the command. When
	// false, only text-level rules were applied.
	Parsed bool `json:"parsed"`
	// Explanation is the model's prose, absent when no model was consulted.
	Explanation string `json:"explanation,omitempty"`
	// Provider and Model name where the explanation came from.
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

// New builds a report from a rule verdict.
func New(command string, v risk.Verdict) Report {
	return Report{
		Command:      command,
		Severity:     v.Severity,
		RuleSeverity: v.Severity,
		Findings:     v.Findings,
		Parsed:       v.Parsed,
	}
}

// WithModelSeverity records the model's rating and applies it as a raise only.
// A model cannot talk a verdict down: the command text is attacker-controlled,
// and a severity the command could argue with would be a way around the rules
// rather than an addition to them.
func (r Report) WithModelSeverity(sev risk.Severity) Report {
	r.ModelSeverity = &sev
	if sev > r.Severity {
		r.Severity = sev
	}
	return r
}

// Verdict is the one-line summary printed above the explanation.
func (r Report) Verdict() string {
	if len(r.Findings) == 0 {
		return r.Severity.String() + " — no rule matched this command."
	}
	return r.Severity.String() + " — " + collapse(r.Findings[0].Reason)
}

// WriteVerdict prints the verdict line. It is called before the explanation is
// requested, because the rules need no model and no network: the part that
// decides is instant, and the part that describes can take its time.
func (r Report) WriteVerdict(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "%s\n", r.Verdict()); err != nil {
		return err
	}

	// Every other rule that fired, so a command caught by several is not
	// reduced to its worst one.
	for _, f := range r.Findings[min(1, len(r.Findings)):] {
		if _, err := fmt.Fprintf(w, "  %s — %s\n", f.Severity, collapse(f.Reason)); err != nil {
			return err
		}
	}

	_, err := fmt.Fprintln(w)
	return err
}

// WriteEscalation prints a note when the model rated the command higher than
// the rules did. It runs after the explanation, because by then the verdict
// line has already been printed.
func (r Report) WriteEscalation(w io.Writer) error {
	if r.ModelSeverity == nil || *r.ModelSeverity <= r.RuleSeverity {
		return nil
	}
	_, err := fmt.Fprintf(w,
		"\nRaised to %s: no rule matched, but %s rated this %s. Rules set the floor, the model can raise it.\n",
		r.Severity, r.Provider, r.ModelSeverity)
	return err
}

// WriteJSON renders the report for another program. The JSON path buffers
// rather than streams, so a failure part-way through never produces a
// half-written object that a caller would try to parse.
func (r Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// collapse turns the folded YAML of a rule reason into a single line.
func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }
