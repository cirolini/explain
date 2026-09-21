// Command eval scores explain's verdicts against labels.
//
// It answers a different question from the rules test table: not "do the rules
// do what they say", but "across commands people and agents actually run, how
// close do the verdicts come to a considered judgement".
//
// The rules-only arm needs no network and no credentials. The --provider arm
// asks a model as well, which costs a request per command.
//
//	go run ./tools/eval                              # rules only
//	go run ./tools/eval --provider anthropic         # rules + a model
//	go run ./tools/eval --provider ... --blind       # the model never sees the rules
//	go run ./tools/eval --markdown                   # tables for docs/results.md
//
// Against a rate-limited free tier, pace the run and keep every case:
//
//	go run ./tools/eval --provider openai-compatible \
//	    --base-url https://api.groq.com/openai/v1 --model openai/gpt-oss-20b \
//	    --delay 15s --out results.json
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/cirolini/explain/internal/config"
	"github.com/cirolini/explain/internal/llm"
	"github.com/cirolini/explain/internal/prompt"
	"github.com/cirolini/explain/internal/report"
	"github.com/cirolini/explain/internal/risk"
	"github.com/openai/openai-go/v3"
	"gopkg.in/yaml.v3"
)

// evalCase is one labelled command.
type evalCase struct {
	Command string `yaml:"command" json:"command"`
	Label   string `yaml:"label" json:"label"`
	Agent   bool   `yaml:"agent" json:"agent"`
}

// result is everything explain concluded about one case, kept separately so
// the rules, the model and the combination can each be scored on their own.
type result struct {
	evalCase
	// Want is the label.
	Want risk.Severity `json:"want"`
	// Rule is what the deterministic rules alone concluded.
	Rule risk.Severity `json:"rule"`
	// Model is the model's own rating, nil when no model was asked or it gave
	// no rating.
	Model *risk.Severity `json:"model,omitempty"`
	// Got is the final verdict: Rule, raised by Model if Model was higher.
	Got risk.Severity `json:"got"`

	// ModelAsked reports whether a model was consulted for this case.
	ModelAsked bool `json:"model_asked"`
	// ModelEmpty reports a model that answered with no visible text at all.
	ModelEmpty bool `json:"model_empty,omitempty"`
}

// agreed reports whether the final verdict matched the label.
func (r result) agreed() bool { return r.Want == r.Got }

// understated reports a command rated less dangerous than its label. These are
// the failures that matter: a guardrail that misses is worse than one that
// nags.
func (r result) understated() bool { return r.Got < r.Want }

// overstated reports a command rated more dangerous than its label. These are
// what get a guardrail switched off.
func (r result) overstated() bool { return r.Got > r.Want }

// options are the command-line settings.
type options struct {
	blind   bool
	delay   time.Duration
	retries int
}

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
		markdown = flag.Bool("markdown", false, "emit Markdown tables")
		out      = flag.String("out", "", "also write every case's result to this JSON file")
		opts     options
	)
	flag.BoolVar(&opts.blind, "blind", false,
		"do not show the model what the rules found, so its rating is its own")
	flag.DurationVar(&opts.delay, "delay", 0, "pause between model requests")
	flag.IntVar(&opts.retries, "retries", 6, "attempts per case when the provider is rate limiting")
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
		arm = fmt.Sprintf("rules + %s", p.Model())
		if opts.blind {
			arm += " (blind)"
		}
	}

	results, err := score(cases, p, opts)
	if err != nil {
		return err
	}

	if *out != "" {
		if err := writeJSON(*out, arm, results); err != nil {
			return err
		}
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
//
// A case the provider cannot answer after every retry fails the whole run
// rather than being skipped. Rate limiting does not fall on cases at random,
// so a run that quietly drops the ones that hit the limit measures a
// different dataset from the one it claims to.
func score(cases []evalCase, p llm.Provider, opts options) ([]result, error) {
	rules := risk.MustDefault()
	out := make([]result, 0, len(cases))

	for i, c := range cases {
		want, err := risk.ParseSeverity(c.Label)
		if err != nil {
			return nil, fmt.Errorf("case %q: %w", c.Command, err)
		}

		v := rules.Evaluate(c.Command)
		res := result{evalCase: c, Want: want, Rule: v.Severity, Got: v.Severity}

		if p != nil {
			if i > 0 && opts.delay > 0 {
				time.Sleep(opts.delay)
			}

			ctx := prompt.Context{}
			if !opts.blind {
				ctx.RuleSeverity = v.Severity.String()
				for _, f := range v.Findings {
					ctx.RuleReasons = append(ctx.RuleReasons, f.Reason)
				}
			}

			res.ModelAsked = true
			text, err := completeWithRetry(p, prompt.Build(c.Command, prompt.EN, ctx), opts)
			switch {
			case errors.Is(err, llm.ErrEmptyExplanation):
				res.ModelEmpty = true
			case err != nil:
				return nil, fmt.Errorf("case %q: %w", c.Command, err)
			default:
				if sev, ok, _ := report.StripSeverityLine(text); ok {
					res.Model = &sev
					// The same rule the binary applies: raise, never lower.
					res.Got = report.New(c.Command, v).WithModelSeverity(sev).Severity
				}
			}
			fmt.Fprintf(os.Stderr, "[%d/%d] %s\n", i+1, len(cases), c.Command)
		}

		out = append(out, res)
	}
	return out, nil
}

// completeWithRetry asks the provider, waiting out rate limits and transient
// server errors. Anything else is returned at once.
func completeWithRetry(p llm.Provider, req prompt.Request, opts options) (string, error) {
	var err error
	for attempt := 1; attempt <= max(1, opts.retries); attempt++ {
		var text string
		text, err = p.Complete(context.Background(), req, io.Discard)
		if err == nil || !transient(err) {
			return text, err
		}

		wait := time.Duration(attempt) * 30 * time.Second
		wait = min(wait, 2*time.Minute)
		fmt.Fprintf(os.Stderr, "  rate limited or unavailable (attempt %d), waiting %s: %v\n", attempt, wait, err)
		time.Sleep(wait)
	}
	return "", fmt.Errorf("gave up after %d attempts: %w", opts.retries, err)
}

// transient reports an error worth retrying: rate limiting or a server fault.
func transient(err error) bool {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 429 || apiErr.StatusCode >= 500
	}
	return false
}

// summary is the headline numbers for the final verdict.
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

// modelSummary scores the model's own ratings, before the rules' floor is
// applied. It is what the model would have concluded on its own.
type modelSummary struct {
	Asked, Rated, Empty, Unmarked   int
	Agreed, Understated, Overstated int
	BelowRules                      int
	BelowRulesCases                 []result
}

func summariseModel(results []result) modelSummary {
	var s modelSummary
	for _, r := range results {
		if !r.ModelAsked {
			continue
		}
		s.Asked++
		switch {
		case r.ModelEmpty:
			s.Empty++
			continue
		case r.Model == nil:
			s.Unmarked++
			continue
		}

		s.Rated++
		m := *r.Model
		switch {
		case m == r.Want:
			s.Agreed++
		case m < r.Want:
			s.Understated++
		default:
			s.Overstated++
		}
		// Every one of these is a case where the model, left to decide, would
		// have rated a command lower than the rules did -- and the floor is why
		// the verdict did not move.
		if m < r.Rule {
			s.BelowRules++
			s.BelowRulesCases = append(s.BelowRulesCases, r)
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
	fmt.Fprintf(&b, "final verdict\n")
	fmt.Fprintf(&b, "  agreed with label   %3d  %s\n", s.Agreed, pct(s.Agreed, s.Total))
	fmt.Fprintf(&b, "  rated too low       %3d  %s\n", s.Understated, pct(s.Understated, s.Total))
	fmt.Fprintf(&b, "  rated too high      %3d  %s\n", s.Overstated, pct(s.Overstated, s.Total))
	fmt.Fprintf(&b, "  agreed, agent-only  %3d  %s\n\n", s.AgentAgreed, pct(s.AgentAgreed, s.AgentTotal))

	if m := summariseModel(results); m.Asked > 0 {
		fmt.Fprintf(&b, "model on its own, before the rules' floor\n")
		fmt.Fprintf(&b, "  gave a rating       %3d  of %d asked (%d empty, %d without a rating)\n",
			m.Rated, m.Asked, m.Empty, m.Unmarked)
		fmt.Fprintf(&b, "  agreed with label   %3d  %s\n", m.Agreed, pct(m.Agreed, m.Rated))
		fmt.Fprintf(&b, "  rated too low       %3d  %s\n", m.Understated, pct(m.Understated, m.Rated))
		fmt.Fprintf(&b, "  rated too high      %3d  %s\n", m.Overstated, pct(m.Overstated, m.Rated))
		fmt.Fprintf(&b, "  below the rules     %3d  (the floor held each time)\n", m.BelowRules)
		for _, r := range m.BelowRulesCases {
			fmt.Fprintf(&b, "    model %-6s rules %-6s  %s\n", *r.Model, r.Rule, r.Command)
		}
		fmt.Fprintln(&b)
	}

	if miss := filter(results, result.understated); len(miss) > 0 {
		fmt.Fprintf(&b, "final verdict rated too low:\n")
		for _, r := range miss {
			fmt.Fprintf(&b, "  %-6s want %-6s  %s\n", r.Got, r.Want, r.Command)
		}
		fmt.Fprintln(&b)
	}
	if over := filter(results, result.overstated); len(over) > 0 {
		fmt.Fprintf(&b, "final verdict rated too high:\n")
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

// writeJSON records every case, so a run that took an hour against a rate
// limit can be re-analysed without being repeated.
func writeJSON(path, arm string, results []result) error {
	data, err := json.MarshalIndent(struct {
		Arm     string   `json:"arm"`
		Results []result `json:"results"`
	}{arm, results}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
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
