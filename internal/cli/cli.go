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

			provider, err := opts.NewProvider(cfg)
			if err != nil {
				return err
			}

			command := strings.Join(args, " ")
			explanation, err := provider.Complete(cmd.Context(), prompt.Build(command))
			if err != nil {
				return err
			}

			_, err = fmt.Fprintf(out, "%s\n", explanation)
			return err
		},
	}

	cmd.Flags().StringVar(&cfg.Model, "model", cfg.Model, "model to use")
	// Everything after the first non-flag argument belongs to the command being
	// explained, so `explain rm -rf /tmp/x` does not trip over -rf.
	cmd.Flags().SetInterspersed(false)

	return cmd
}
