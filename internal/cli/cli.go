// Package cli assembles explain's cobra command.
package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/cirolini/explain/internal/config"
	"github.com/cirolini/explain/internal/llm"
	"github.com/cirolini/explain/internal/prompt"
	"github.com/cirolini/explain/internal/report"
	"github.com/cirolini/explain/internal/risk"
	"github.com/spf13/cobra"
)

// ProviderFactory builds a provider from the configuration left after flags
// have been applied. It is injected so tests can substitute llm.Fake.
type ProviderFactory func(config.Config) (llm.Provider, error)

// Options configures NewCommand.
type Options struct {
	// Config is the configuration resolved before flag parsing.
	Config config.Config
	// NewProvider builds the provider to run against.
	NewProvider ProviderFactory
	// Out receives the explanation. Defaults to the command's own stdout.
	Out io.Writer
	// Version is reported by --version.
	Version string
	// Rules are the deterministic risk rules. Defaults to the built-in set.
	Rules *risk.RuleSet
}

// NewCommand returns the root `explain` command.
func NewCommand(opts Options) *cobra.Command {
	cfg := opts.Config
	if opts.Rules == nil {
		opts.Rules = risk.MustDefault()
	}
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "explain <command>",
		Short: "Explain a shell command before you run it",
		Long: "explain describes what a shell command does, what it touches, and\n" +
			"whether its effects can be undone -- without running it.\n\n" +
			"The command may be quoted as a single argument or given as trailing\n" +
			"words:\n\n  explain \"ls -lrth\"\n  explain ls -lrth",
		Args:    cobra.MinimumNArgs(1),
		Version: opts.Version,
		// explain never runs the command it is given; cobra should not try to
		// interpret the command's own flags as its own.
		DisableFlagsInUseLine: true,
		SilenceUsage:          true,
		SilenceErrors:         true,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := opts.Out
			if out == nil {
				out = cmd.OutOrStdout()
			}

			if err := cfg.Resolve(); err != nil {
				return err
			}

			command := strings.Join(args, " ")

			// The rules decide, and they need no model and no network. Do that
			// first, so the verdict exists even if everything after fails.
			rep := report.New(command, opts.Rules.Evaluate(command))

			provider, err := opts.NewProvider(cfg)
			if err != nil {
				return err
			}
			rep.Provider, rep.Model = provider.Name(), provider.Model()

			if asJSON {
				return runJSON(cmd.Context(), provider, rep, cfg, out)
			}
			return runText(cmd.Context(), provider, rep, cfg, out)
		},
	}

	f := cmd.Flags()
	f.StringVar(&cfg.Provider, "provider", cfg.Provider,
		"provider: openai, anthropic or openai-compatible")
	f.StringVar(&cfg.Model, "model", cfg.Model,
		"model to use (default: the provider's own default)")
	f.StringVar(&cfg.BaseURL, "base-url", cfg.BaseURL,
		"OpenAI-compatible endpoint, e.g. http://localhost:11434/v1 for Ollama")
	f.StringVar(&cfg.Lang, "lang", cfg.Lang, "explanation language: en or pt")
	f.BoolVar(&asJSON, "json", false, "print the verdict and explanation as JSON")
	// Everything after the first non-flag argument belongs to the command being
	// explained, so `explain rm -rf /tmp/x` does not trip over -rf.
	f.SetInterspersed(false)

	return cmd
}

// runText prints the verdict, then streams the explanation beneath it.
//
// The ordering is the point of the tool: the verdict comes from rules, so it
// is on screen before the model has said anything. If the model then rates the
// command higher than the rules did, that is noted after the explanation --
// it can raise the verdict, never lower it.
func runText(ctx context.Context, p llm.Provider, rep report.Report, cfg config.Config, out io.Writer) error {
	if err := rep.WriteVerdict(out); err != nil {
		return err
	}

	filter := report.NewSeverityFilter(out)
	_, err := p.Complete(ctx, buildPrompt(rep, cfg), filter)
	if err != nil {
		return err
	}
	if err := filter.Flush(); err != nil {
		return err
	}

	if sev, ok := filter.Severity(); ok {
		rep = rep.WithModelSeverity(sev)
	}
	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	return rep.WriteEscalation(out)
}

// runJSON buffers the explanation and prints one object. Nothing is streamed:
// a failure part-way through a stream would leave a half-written object that a
// caller would try to parse.
func runJSON(ctx context.Context, p llm.Provider, rep report.Report, cfg config.Config, out io.Writer) error {
	text, err := p.Complete(ctx, buildPrompt(rep, cfg), io.Discard)
	if err != nil {
		return err
	}

	sev, ok, explanation := report.StripSeverityLine(text)
	rep.Explanation = explanation
	if ok {
		rep = rep.WithModelSeverity(sev)
	}
	return rep.WriteJSON(out)
}

// buildPrompt assembles the model request from the rule findings.
func buildPrompt(rep report.Report, cfg config.Config) prompt.Request {
	reasons := make([]string, 0, len(rep.Findings))
	for _, f := range rep.Findings {
		reasons = append(reasons, f.Reason)
	}

	return prompt.Build(rep.Command, prompt.Lang(cfg.Lang), prompt.Context{
		RuleSeverity: rep.RuleSeverity.String(),
		RuleReasons:  reasons,
	})
}
