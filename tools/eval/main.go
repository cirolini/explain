// Command eval scores explain's verdicts against human labels.
//
// It answers a different question from the rules test table: not "do the rules
// do what they say", but "across commands people and agents actually run, how
// close do the verdicts come to a human's judgement".
//
// The rules-only arm needs no network and no credentials. The --provider arm
// asks a model as well, which costs a request per command.
//
//	go run ./tools/eval                      # rules only
//	go run ./tools/eval --provider anthropic # rules + a model
//	go run ./tools/eval --markdown           # emit the table for docs/results.md
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/cirolini/explain/internal/config"
	"github.com/cirolini/explain/internal/llm"
	"github.com/cirolini/explain/internal/prompt"
	"github.com/cirolini/explain/internal/report"
	"github.com/cirolini/explain/internal/risk"
	"gopkg.in/yaml.v3"
)

// evalCase is one labelled command.
type evalCase struct {
	Command string `yaml:"command"`
	Label   string `yaml:"label"`
	Agent   bool   `yaml:"agent"`
}

// result pairs a case with what explain said about it.
type result struct {
	evalCase
	Want risk.Severity
	Got  risk.Severity
}

// agreed reports whether the verdict matched the label.
func (r result) agreed() bool { return r.Want == r.Got }

// understated reports a command rated less dangerous than its label. These are
// the failures that matter: a guardrail that misses is worse than one that
// nags.
func (r result) understated() bool { return r.Got < r.Want }

// overstated reports a command rated more dangerous than its label. These are
// what get a guardrail switched off.
func (r result) overstated() bool { return r.Got > r.Want }

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "eval: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		dataset  = flag.String("dataset", "tools/eval/dataset.yaml", "labelled commands")
		provider = flag.String("provider", "", "also ask this provider (costs one request per command)")
		model    = flag.String("model", "", "model to use with --provider")
		baseURL  = flag.String("base-url", "", "OpenAI-compatible endpoint")
		markdown = flag.Bool("markdown", false, "emit a Markdown table")
	)
	flag.Parse()

	cases, err := load(*dataset)
	if err != nil {
		return err
	}

	arm := "rules only"
	var p llm.Provider
	if *provider != "" {
		cfg, err := providerConfig(*provider, *model, *baseURL)
		if err != nil {
			return err
		}
		if p, err = newProvider(cfg); err != nil {
			return err
		}
		arm = fmt.Sprintf("rules + %s/%s", p.Name(), p.Model())
	}

	results, err := score(cases, p)
	if err != nil {
		return err
	}

	if *markdown {
		return writeMarkdown(os.Stdout, arm, results)
	}
	return writeText(os.Stdout, arm, results)
}

// load reads the dataset.
func load(path string) ([]evalCase, error) {
	data, err := os.ReadFile(path) //nolint:gosec // a path the operator passed
	if err != nil {
		return nil, err
	}

	var file struct {
		Cases []evalCase `yaml:"cases"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if len(file.Cases) == 0 {
		return nil, fmt.Errorf("%s has no cases", path)
	}
	return file.Cases, nil
}

// score evaluates every case. When p is nil, only the rules run.
func score(cases []evalCase, p llm.Provider) ([]result, error) {
	rules := risk.MustDefault()
	out := make([]result, 0, len(cases))

	for _, c := range cases {
		want, err := risk.ParseSeverity(c.Label)
		if err != nil {
			return nil, fmt.Errorf("case %q: %w", c.Command, err)
		}

		rep := report.New(c.Command, rules.Evaluate(c.Command))

		if p != nil {
			reasons := make([]string, 0, len(rep.Findings))
			for _, f := range rep.Findings {
				reasons = append(reasons, f.Reason)
			}
			text, err := p.Complete(context.Background(), prompt.Build(
				c.Command, prompt.EN,
				prompt.Context{RuleSeverity: rep.RuleSeverity.String(), RuleReasons: reasons},
			), io.Discard)
			if err != nil {
				return nil, fmt.Errorf("case %q: %w", c.Command, err)
			}
			if sev, ok, _ := report.StripSeverityLine(text); ok {
				rep = rep.WithModelSeverity(sev)
			}
		}

		out = append(out, result{evalCase: c, Want: want, Got: rep.Severity})
	}
	return out, nil
}

// summary is the headline numbers for one arm.
type summary struct {
	Total, Agreed, Understated, Overstated int
	AgentTotal, AgentAgreed                int
}

func summarise(results []result) summary {
	var s summary
	for _, r := range results {
		s.Total++
		if r.Agent {
			s.AgentTotal++
		}
		switch {
		case r.agreed():
			s.Agreed++
			if r.Agent {
				s.AgentAgreed++
			}
		case r.understated():
			s.Understated++
		case r.overstated():
			s.Overstated++
		}
	}
	return s
}

func pct(n, total int) string {
	if total == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.0f%%", 100*float64(n)/float64(total))
}

func writeText(w io.Writer, arm string, results []result) error {
	s := summarise(results)
	var b strings.Builder

	fmt.Fprintf(&b, "arm: %s\n", arm)
	fmt.Fprintf(&b, "cases: %d (%d agent-typical)\n\n", s.Total, s.AgentTotal)
	fmt.Fprintf(&b, "  agreed with label   %3d  %s\n", s.Agreed, pct(s.Agreed, s.Total))
	fmt.Fprintf(&b, "  rated too low       %3d  %s\n", s.Understated, pct(s.Understated, s.Total))
	fmt.Fprintf(&b, "  rated too high      %3d  %s\n", s.Overstated, pct(s.Overstated, s.Total))
	fmt.Fprintf(&b, "  agreed, agent-only  %3d  %s\n\n", s.AgentAgreed, pct(s.AgentAgreed, s.AgentTotal))

	if miss := filter(results, result.understated); len(miss) > 0 {
		fmt.Fprintf(&b, "rated too low (a guardrail that misses is worse than one that nags):\n")
		for _, r := range miss {
			fmt.Fprintf(&b, "  %-6s want %-6s  %s\n", r.Got, r.Want, r.Command)
		}
		fmt.Fprintln(&b)
	}
	if over := filter(results, result.overstated); len(over) > 0 {
		fmt.Fprintf(&b, "rated too high (false positives are what get a guardrail switched off):\n")
		for _, r := range over {
			fmt.Fprintf(&b, "  %-6s want %-6s  %s\n", r.Got, r.Want, r.Command)
		}
	}

	_, err := io.WriteString(w, b.String())
	return err
}

func writeMarkdown(w io.Writer, arm string, results []result) error {
	s := summarise(results)
	var b strings.Builder

	fmt.Fprintf(&b, "| Arm | Cases | Agreed | Rated too low | Rated too high | Agreed (agent-typical) |\n")
	fmt.Fprintf(&b, "| --- | ---: | ---: | ---: | ---: | ---: |\n")
	fmt.Fprintf(&b, "| %s | %d | %d (%s) | %d (%s) | %d (%s) | %d/%d (%s) |\n",
		arm, s.Total,
		s.Agreed, pct(s.Agreed, s.Total),
		s.Understated, pct(s.Understated, s.Total),
		s.Overstated, pct(s.Overstated, s.Total),
		s.AgentAgreed, s.AgentTotal, pct(s.AgentAgreed, s.AgentTotal))

	fmt.Fprintf(&b, "\n### Disagreements — %s\n\n", arm)
	fmt.Fprintf(&b, "| Command | Label | Verdict | |\n| --- | --- | --- | --- |\n")

	bad := append(filter(results, result.understated), filter(results, result.overstated)...)
	sort.SliceStable(bad, func(i, j int) bool { return bad[i].Want > bad[j].Want })
	for _, r := range bad {
		note := "rated too high"
		if r.understated() {
			note = "**rated too low**"
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n",
			strings.ReplaceAll(r.Command, "|", `\|`), r.Want, r.Got, note)
	}

	_, err := io.WriteString(w, b.String())
	return err
}

func filter(results []result, keep func(result) bool) []result {
	var out []result
	for _, r := range results {
		if keep(r) {
			out = append(out, r)
		}
	}
	return out
}

// providerConfig resolves the flags into a configuration.
func providerConfig(provider, model, baseURL string) (config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return cfg, err
	}
	cfg.Provider, cfg.BaseURL = provider, baseURL
	if model != "" {
		cfg.Model = model
	}
	return cfg, cfg.Resolve()
}

// newProvider mirrors the binary's own selection, so the eval measures what
// ships rather than a parallel implementation.
func newProvider(cfg config.Config) (llm.Provider, error) {
	opts := llm.Options{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, Model: cfg.Model}
	switch cfg.Provider {
	case config.ProviderOpenAI, config.ProviderCompatible:
		return llm.NewOpenAI(opts)
	case config.ProviderAnthropic:
		return llm.NewAnthropic(opts)
	default:
		return nil, fmt.Errorf("no adapter for provider %q", cfg.Provider)
	}
}
