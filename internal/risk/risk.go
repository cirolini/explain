package risk

import "sort"

// Finding is one rule that fired.
type Finding struct {
	// RuleID is the id from rules.yaml.
	RuleID string `json:"rule"`
	// Severity is the verdict this rule carries.
	Severity Severity `json:"severity"`
	// Reason explains what the user is being warned about.
	Reason string `json:"reason"`
}

// Verdict is the result of evaluating a command against the rules.
type Verdict struct {
	// Severity is the highest severity among the findings, or Low if none
	// fired.
	Severity Severity `json:"severity"`
	// Findings are the rules that fired, most severe first.
	Findings []Finding `json:"findings"`
	// Parsed reports whether the shell parser understood the command. When
	// false only text-level rules were applied, so the verdict is a floor
	// rather than a full assessment.
	Parsed bool `json:"parsed"`
}

// UnparseableRuleID is the id reported when the shell parser could not read
// the command.
const UnparseableRuleID = "unparseable-command"

// Evaluate classifies a command. It is pure: no network, no model, no
// execution. The same command always yields the same verdict, which is what
// makes it something a hook can depend on.
func (rs *RuleSet) Evaluate(command string) Verdict {
	line := Parse(command)
	v := Verdict{Parsed: line.Parsed}

	// A line explain could not parse cannot be called safe. Only text-level
	// rules ran, so a confident LOW here would be a way past the guardrail:
	// craft something this parser rejects but a shell accepts, and a hook that
	// trusts LOW waves it through. Say so instead, at a level that asks.
	if !line.Parsed {
		v.Findings = append(v.Findings, Finding{
			RuleID:   UnparseableRuleID,
			Severity: Medium,
			Reason: "explain could not parse this command line, so only text-level " +
				"checks were applied. Read it yourself before running it.",
		})
		v.Severity = Medium
	}

	for _, r := range rs.Rules {
		if !r.matches(line) {
			continue
		}
		v.Findings = append(v.Findings, Finding{
			RuleID:   r.ID,
			Severity: r.Severity,
			Reason:   r.Reason,
		})
		if r.Severity > v.Severity {
			v.Severity = r.Severity
		}
	}

	// Most severe first; ties keep the order they appear in rules.yaml, so the
	// output is stable across runs.
	sort.SliceStable(v.Findings, func(i, j int) bool {
		return v.Findings[i].Severity > v.Findings[j].Severity
	})

	return v
}

// AtLeast raises a verdict to sev, and never lowers it. This is the only way a
// model's opinion enters the result: it may argue a command is more dangerous
// than the rules found, never less. The command text is attacker-controlled,
// so a model that could talk the severity down would be a way around the
// rules rather than an addition to them.
func (v Verdict) AtLeast(sev Severity) Verdict {
	if sev > v.Severity {
		v.Severity = sev
	}
	return v
}
