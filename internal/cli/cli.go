// Package cli assembles explain's cobra command.
package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/cirolini/explain/internal/config"
	"github.com/cirolini/explain/internal/llm"
	"github.com/cirolini/explain/internal/prompt"
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
}

// NewCommand returns the root `explain` command.
func NewCommand(opts Options) *cobra.Command {
	cfg := opts.Config

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

			provider, err := opts.NewProvider(cfg)
			if err != nil {
				return err
			}

			command := strings.Join(args, " ")
			req := prompt.Build(command, prompt.Lang(cfg.Lang))

			// The explanation streams to out as it arrives, so the terminal
			// starts filling immediately rather than after the whole response.
			if _, err := provider.Complete(cmd.Context(), req, out); err != nil {
				return err
			}

			_, err = fmt.Fprintln(out)
			return err
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
	// Everything after the first non-flag argument belongs to the command being
	// explained, so `explain rm -rf /tmp/x` does not trip over -rf.
	f.SetInterspersed(false)

	return cmd
}
