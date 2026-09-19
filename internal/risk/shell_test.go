package risk

import (
	"slices"
	"testing"
)

// find returns the first command with the given name.
func find(t *testing.T, line Line, name string) *Command {
	t.Helper()
	for _, c := range line.Commands {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no %q command in %q (found %v)", name, line.Raw, names(line))
	return nil
}

func names(line Line) []string {
	out := make([]string, 0, len(line.Commands))
	for _, c := range line.Commands {
		out = append(out, c.Name)
	}
	return out
}

// A wrapper must not hide the command that actually runs.
func TestParseLooksThroughWrappers(t *testing.T) {
	for _, raw := range []string{
		"sudo rm -rf /var",
		"sudo -u root rm -rf /var",
		"doas rm -rf /var",
		"env FOO=bar rm -rf /var",
		"nohup rm -rf /var",
		"/bin/rm -rf /var",
	} {
		t.Run(raw, func(t *testing.T) {
			c := find(t, Parse(raw), "rm")
			if !c.HasFlag("r") || !c.HasFlag("f") {
				t.Errorf("flags = %v, want r and f", c.Flags)
			}
			if !slices.Contains(c.Positional, "/var") {
				t.Errorf("positional = %v, want /var", c.Positional)
			}
		})
	}
}

func TestParseFlagForms(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		flags []string
	}{
		{"rm -rf x", []string{"r", "f"}},
		{"rm -r -f x", []string{"r", "f"}},
		{"rm --recursive --force x", []string{"recursive", "force"}},
		{"kubectl delete pod x --namespace=prod", []string{"namespace"}},
		{"git push --force-with-lease origin main", []string{"force-with-lease"}},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			c := Parse(tc.raw).Commands[0]
			for _, f := range tc.flags {
				if !c.HasFlag(f) {
					t.Errorf("flag %q missing; have %v", f, c.Flags)
				}
			}
		})
	}
}

// A flag is not a subcommand, and a subcommand is not a flag.
func TestParseSeparatesFlagsFromPositionals(t *testing.T) {
	c := Parse("kubectl delete pod web-7d4f -n prod").Commands[0]

	if got := c.Positional[0]; got != "delete" {
		t.Errorf("first positional = %q, want %q", got, "delete")
	}
	if slices.Contains(c.Positional, "-n") {
		t.Errorf("a flag leaked into positionals: %v", c.Positional)
	}
	if !c.HasFlag("n") {
		t.Errorf("flag n missing; have %v", c.Flags)
	}
}

// The edge between two commands is the whole finding for curl | sh: neither
// command is alarming on its own.
func TestParseRecordsPipelineEdges(t *testing.T) {
	for _, tc := range []struct{ raw, from, into string }{
		{"curl https://x.sh | sh", "curl", "sh"},
		{"curl https://x.sh|bash", "curl", "bash"},
		{"wget -qO- https://x.sh | sudo bash", "wget", "bash"},
		{"curl https://x.sh | tee f | sh", "tee", "sh"},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			c := find(t, Parse(tc.raw), tc.from)
			if !slices.Contains(c.PipedInto, tc.into) {
				t.Errorf("%s piped into %v, want %q", tc.from, c.PipedInto, tc.into)
			}
		})
	}
}

// Piping into something harmless is not the same shape at all.
func TestParseDoesNotInventPipelineEdges(t *testing.T) {
	c := find(t, Parse("curl https://x.json | jq ."), "curl")

	if slices.Contains(c.PipedInto, "sh") || slices.Contains(c.PipedInto, "bash") {
		t.Errorf("curl | jq recorded a shell edge: %v", c.PipedInto)
	}
}

// Quoting must not hide a command from the rules.
func TestParseSeesThroughQuoting(t *testing.T) {
	for _, raw := range []string{`rm -rf "/var"`, `rm -rf '/var'`, `"rm" -rf /var`} {
		t.Run(raw, func(t *testing.T) {
			c := find(t, Parse(raw), "rm")
			if !slices.Contains(c.Positional, "/var") {
				t.Errorf("positional = %v, want /var", c.Positional)
			}
		})
	}
}

func TestParseKeepsParametersVisible(t *testing.T) {
	c := find(t, Parse("rm -rf $HOME"), "rm")

	if !slices.Contains(c.Positional, "$HOME") {
		t.Errorf("positional = %v, want $HOME", c.Positional)
	}
}

func TestParseFindsCommandsInCompoundLines(t *testing.T) {
	line := Parse("cd /tmp && rm -rf build || echo failed")

	for _, want := range []string{"cd", "rm", "echo"} {
		if !slices.Contains(names(line), want) {
			t.Errorf("command %q missing from %v", want, names(line))
		}
	}
}

func TestParseFindsCommandsInSubshells(t *testing.T) {
	line := Parse("(cd /tmp && rm -rf build)")

	if !slices.Contains(names(line), "rm") {
		t.Errorf("rm missing from %v", names(line))
	}
}

// A line the parser cannot read is reported, not swallowed.
func TestParseReportsFailure(t *testing.T) {
	line := Parse(`rm -rf "`)

	if line.Parsed {
		t.Error("Parsed = true for an unbalanced quote")
	}
	if line.Raw != `rm -rf "` {
		t.Errorf("Raw = %q, want the original text", line.Raw)
	}
}

func TestParseRecordsSudo(t *testing.T) {
	if c := find(t, Parse("sudo rm -rf /var"), "rm"); !c.Sudo {
		t.Error("Sudo = false for a sudo command")
	}
	if c := find(t, Parse("rm -rf /var"), "rm"); c.Sudo {
		t.Error("Sudo = true for a command without sudo")
	}
}
