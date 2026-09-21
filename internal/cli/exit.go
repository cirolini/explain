package cli

import (
	"fmt"

	"github.com/cirolini/explain/internal/risk"
)

// Exit codes. A verdict is not a failure, so the risk levels take the low
// numbers and anything that actually went wrong is reported above them. A
// caller that cannot tell "this command is dangerous" from "explain could not
// run" would have to treat both the same, which in a guardrail means either
// blocking on outages or ignoring real findings.
const (
	// ExitLow means no rule matched.
	ExitLow = 0
	// ExitMedium means the command deserves a look.
	ExitMedium = 1
	// ExitHigh means the command is destructive or runs unreviewed code.
	ExitHigh = 2
	// ExitError means explain itself failed: bad usage, no API key, no
	// network. It says nothing about the command.
	ExitError = 3
)

// ExitCodeFor maps a verdict to its exit code.
func ExitCodeFor(sev risk.Severity) int {
	switch sev {
	case risk.High:
		return ExitHigh
	case risk.Medium:
		return ExitMedium
	default:
		return ExitLow
	}
}

// VerdictError carries a risk verdict out through cobra's error return so that
// main can exit with the matching status. It is not a failure, and printing it
// as one would be wrong, so its message is empty.
type VerdictError struct {
	Severity risk.Severity
}

// Error implements error.
func (e VerdictError) Error() string { return "" }

// Code returns the exit status for this verdict.
func (e VerdictError) Code() int { return ExitCodeFor(e.Severity) }

// ensure VerdictError reads as intended at compile time.
var _ error = VerdictError{}

// String implements fmt.Stringer, for tests and debugging.
func (e VerdictError) String() string { return fmt.Sprintf("verdict %s", e.Severity) }
