package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/cirolini/explain/internal/config"
	"github.com/cirolini/explain/internal/report"
	"github.com/spf13/cobra"
)

// newCheckCommand builds `explain check`, the form meant for scripts and
// hooks.
//
// It does not consult a model unless asked. The rules need no network, no API
// key and no credit, and they answer in microseconds -- which matters because
// this sits in front of every command an agent wants to run. A guardrail that
// costs a round trip per command is a guardrail someone switches off, and one
// that fails when the network does is worse than none.
func newCheckCommand(opts Options, cfg *config.Config) *cobra.Command {
	var (
		asJSON    bool
		withModel bool
	)

	cmd := &cobra.Command{
		Use:   "check <command>",
		Short: "Classify a command's risk and exit with a matching status",
		Long: fmt.Sprintf(`check classifies a shell command and exits with a status a script can branch on:

  %d  LOW     no rule matched
  %d  MEDIUM  changes state, or deserves a look
  %d  HIGH    destroys data, or runs unreviewed code
  %d  error   explain itself failed, and said nothing about the command

By default no model is consulted: the verdict comes from rules alone, which
need no network and no API key. Pass --explain to add a model's description,
which can raise the verdict but never lower it.`,
			ExitLow, ExitMedium, ExitHigh, ExitError),
		Args:                  cobra.MinimumNArgs(1),
		DisableFlagsInUseLine: true,
		SilenceUsage:          true,
		SilenceErrors:         true,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := opts.Out
			if out == nil {
				out = cmd.OutOrStdout()
			}

			command := strings.Join(args, " ")
			rep := report.New(command, opts.Rules.Evaluate(command))

			if withModel {
				if err := cfg.Resolve(); err != nil {
					return err
				}
				provider, err := opts.NewProvider(*cfg)
				if err != nil {
					return err
				}
				rep.Provider, rep.Model = provider.Name(), provider.Model()

				text, err := provider.Complete(cmd.Context(), buildPrompt(rep, *cfg), io.Discard)
				if err != nil {
					return err
				}
				sev, ok, explanation := report.StripSeverityLine(text)
				rep.Explanation = explanation
				if ok {
					rep = rep.WithModelSeverity(sev)
				}
			}

			if err := writeCheck(out, rep, asJSON); err != nil {
				return err
			}
			return VerdictError{Severity: rep.Severity}
		},
	}

	f := cmd.Flags()
	f.BoolVar(&asJSON, "json", false, "print the verdict as JSON")
	f.BoolVar(&withModel, "explain", false,
		"also ask a model to describe the command (costs a round trip)")
	f.SetInterspersed(false)

	return cmd
}

// writeCheck prints the verdict in the requested form. The text form is one
// line, because a hook that shows it to a person has one line of attention.
func writeCheck(w io.Writer, rep report.Report, asJSON bool) error {
	if asJSON {
		return rep.WriteJSON(w)
	}

	if _, err := fmt.Fprintln(w, rep.Verdict()); err != nil {
		return err
	}
	if rep.Explanation != "" {
		_, err := fmt.Fprintf(w, "\n%s\n", rep.Explanation)
		return err
	}
	return nil
}
