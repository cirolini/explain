// Package risk classifies shell commands by how much damage running them
// could do.
//
// The verdict comes from deterministic rules held as data, never from a model.
// That is the whole point: the command text is attacker-controlled, and a
// classifier a command can argue with is not a guardrail. A model may add
// explanation, and may raise a verdict, but it can never lower one.
package risk

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed rules.yaml
var rulesYAML []byte

// Severity is how much damage a command could do.
type Severity int

// Severity levels, ordered so that they can be compared.
const (
	// Low is a command with no destructive effect this tool recognises.
	Low Severity = iota
	// Medium is a command that changes state in a way that is recoverable,
	// or that needs a look before it runs.
	Medium
	// High is a command that destroys data, executes untrusted input, or
	// cannot be undone.
	High
)

// String implements fmt.Stringer.
func (s Severity) String() string {
	switch s {
	case High:
		return "HIGH"
	case Medium:
		return "MEDIUM"
	default:
		return "LOW"
	}
}

// ParseSeverity converts a severity name, as written in rules.yaml or returned
// by a model.
func ParseSeverity(s string) (Severity, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "low":
		return Low, nil
	case "medium":
		return Medium, nil
	case "high":
		return High, nil
	default:
		return Low, fmt.Errorf("unknown severity %q", s)
	}
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (s *Severity) UnmarshalYAML(node *yaml.Node) error {
	parsed, err := ParseSeverity(node.Value)
	if err != nil {
		return err
	}
	*s = parsed
	return nil
}

// MarshalJSON renders the severity as its name.
func (s Severity) MarshalJSON() ([]byte, error) { return []byte(`"` + s.String() + `"`), nil }

// Rule is one deterministic check.
type Rule struct {
	// ID names the rule, and is what --json reports.
	ID string `yaml:"id"`
	// Severity is the verdict this rule carries when it matches.
	Severity Severity `yaml:"severity"`
	// Reason explains, in one sentence, what the user is being warned about.
	Reason string `yaml:"reason"`
	// Match describes the commands this rule fires on.
	Match Match `yaml:"match"`
}

// Match is the condition a rule fires on. Every field that is set must hold,
// so a rule with both Command and Flags requires both.
type Match struct {
	// Command matches any of these executable names.
	Command []string `yaml:"command"`
	// Subcommand requires the first positional argument to be one of these.
	Subcommand []string `yaml:"subcommand"`
	// Flags requires every one of these flags to be present.
	Flags []string `yaml:"flags"`
	// AnyFlag requires at least one of these flags to be present.
	AnyFlag []string `yaml:"any_flag"`
	// MissingFlags fires only when none of these flags is present, for rules
	// about an omission -- a delete with no namespace, say.
	MissingFlags []string `yaml:"missing_flags"`
	// ArgPattern requires at least one positional argument to match one of
	// these regular expressions.
	ArgPattern []string `yaml:"arg_pattern"`
	// PipeInto requires the command's output to be piped into one of these.
	PipeInto []string `yaml:"pipe_into"`
	// TextPattern matches case-insensitively against the whole line. It is the
	// fallback for things that are not shell structure -- SQL in an argument,
	// a credential pasted on the command line -- and the only kind of rule
	// that still applies when the shell parser cannot read the line.
	TextPattern []string `yaml:"text_pattern"`

	// Compiled forms of the pattern fields.
	argRe  []*regexp.Regexp
	textRe []*regexp.Regexp
}

// isTextOnly reports whether the match depends solely on the raw text, and so
// can be evaluated without a successful parse.
func (m Match) isTextOnly() bool {
	return len(m.TextPattern) > 0 &&
		len(m.Command) == 0 && len(m.Subcommand) == 0 && len(m.Flags) == 0 &&
		len(m.AnyFlag) == 0 && len(m.MissingFlags) == 0 &&
		len(m.ArgPattern) == 0 && len(m.PipeInto) == 0
}

// RuleSet is the ordered collection of rules explain evaluates.
type RuleSet struct {
	Rules []*Rule `yaml:"rules"`
}

// Default returns the rules compiled into the binary.
func Default() (*RuleSet, error) { return Load(rulesYAML) }

// MustDefault returns the built-in rules, panicking if they are invalid. The
// rules ship inside the binary, so a failure here is a build-time mistake, not
// something a user can cause.
func MustDefault() *RuleSet {
	rs, err := Default()
	if err != nil {
		panic("risk: built-in rules are invalid: " + err.Error())
	}
	return rs
}

// Load parses and validates a rule set.
func Load(data []byte) (*RuleSet, error) {
	var rs RuleSet
	if err := yaml.Unmarshal(data, &rs); err != nil {
		return nil, fmt.Errorf("parsing rules: %w", err)
	}
	if len(rs.Rules) == 0 {
		return nil, fmt.Errorf("rule set is empty")
	}

	seen := map[string]bool{}
	for i, r := range rs.Rules {
		switch {
		case r.ID == "":
			return nil, fmt.Errorf("rule %d has no id", i)
		case seen[r.ID]:
			return nil, fmt.Errorf("duplicate rule id %q", r.ID)
		case r.Reason == "":
			return nil, fmt.Errorf("rule %q has no reason", r.ID)
		}
		seen[r.ID] = true

		if err := r.Match.compile(); err != nil {
			return nil, fmt.Errorf("rule %q: %w", r.ID, err)
		}
	}
	return &rs, nil
}

// compile builds the regular expressions a match needs.
func (m *Match) compile() error {
	for _, p := range m.ArgPattern {
		re, err := regexp.Compile(p)
		if err != nil {
			return fmt.Errorf("arg_pattern %q: %w", p, err)
		}
		m.argRe = append(m.argRe, re)
	}
	for _, p := range m.TextPattern {
		re, err := regexp.Compile("(?i)" + p)
		if err != nil {
			return fmt.Errorf("text_pattern %q: %w", p, err)
		}
		m.textRe = append(m.textRe, re)
	}
	if len(m.Command) == 0 && len(m.textRe) == 0 {
		return fmt.Errorf("match has neither command nor text_pattern")
	}
	return nil
}
