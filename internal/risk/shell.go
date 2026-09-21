package risk

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// wrappers run another command and should be looked through, so that
// `sudo rm -rf /` is judged as `rm -rf /` rather than as a call to sudo. The
// value is the set of that wrapper's own flags which consume the next word:
// without them, `sudo -u root rm -rf /var` reads as a call to `root`.
var wrappers = map[string]map[string]bool{
	"sudo": {
		"u": true, "g": true, "p": true, "C": true, "h": true, "r": true,
		"t": true, "U": true, "user": true, "group": true, "prompt": true,
		"host": true, "role": true, "type": true, "other-user": true,
		"close-from": true,
	},
	"doas":    {"u": true, "C": true, "L": true},
	"env":     {"u": true, "C": true, "S": true, "unset": true, "chdir": true},
	"nice":    {"n": true, "adjustment": true},
	"xargs":   {"n": true, "P": true, "I": true, "i": true, "d": true, "a": true, "E": true, "L": true, "s": true},
	"time":    {"o": true, "f": true},
	"nohup":   {},
	"command": {},
}

// Command is one simple command inside a shell line.
type Command struct {
	// Name is the executable, after any wrapper such as sudo is stripped.
	Name string
	// Args are every argument after the name, exactly as written.
	Args []string
	// Positional are the arguments that are not flags.
	Positional []string
	// Flags holds every flag seen, normalised: bundled short flags are split,
	// so -rf yields "r" and "f", and --namespace=x yields "namespace".
	Flags map[string]bool
	// PipedInto names the commands this one's output is piped into.
	PipedInto []string
	// Sudo reports whether the command was run through a privilege wrapper.
	Sudo bool
}

// HasFlag reports whether the command carries the given flag, which may be a
// short letter ("f") or a long name ("force").
func (c Command) HasFlag(name string) bool { return c.Flags[strings.TrimLeft(name, "-")] }

// Line is a parsed shell line.
type Line struct {
	// Raw is the command text exactly as it was given.
	Raw string
	// Commands are every simple command in the line, in source order.
	Commands []*Command
	// Parsed reports whether the shell parser understood the line. When false,
	// Commands is empty and only text-level rules can be applied.
	Parsed bool
}

// Parse breaks a shell line into its commands. A line the parser cannot
// understand is not an error: explain must still say something useful about
// it, so Parse returns a Line with Parsed false and the raw text intact, and
// the caller falls back to text-level rules.
func Parse(raw string) Line {
	line := Line{Raw: raw}

	f, err := syntax.NewParser().Parse(strings.NewReader(raw), "")
	if err != nil {
		return line
	}
	line.Parsed = true

	for _, stmt := range f.Stmts {
		line.walk(stmt)
	}
	return line
}

// walk descends a statement, recording commands and pipeline edges.
func (l *Line) walk(stmt *syntax.Stmt) {
	if stmt == nil {
		return
	}

	switch cmd := stmt.Cmd.(type) {
	case *syntax.CallExpr:
		if c := newCommand(cmd); c != nil {
			l.Commands = append(l.Commands, c)
		}

	case *syntax.BinaryCmd:
		if cmd.Op == syntax.Pipe || cmd.Op == syntax.PipeAll {
			l.walkPipe(cmd)
			return
		}
		l.walk(cmd.X)
		l.walk(cmd.Y)

	case *syntax.Subshell:
		for _, s := range cmd.Stmts {
			l.walk(s)
		}
	case *syntax.Block:
		for _, s := range cmd.Stmts {
			l.walk(s)
		}
	case *syntax.IfClause:
		for _, s := range append(cmd.Cond, cmd.Then...) {
			l.walk(s)
		}
	case *syntax.WhileClause:
		for _, s := range append(cmd.Cond, cmd.Do...) {
			l.walk(s)
		}
	case *syntax.ForClause:
		for _, s := range cmd.Do {
			l.walk(s)
		}
	case *syntax.FuncDecl:
		l.walk(cmd.Body)
	}
}

// walkPipe records a pipeline, linking each stage to the one it feeds.
// `curl x | sh` is the shape that matters most here: neither command is
// alarming alone, and only the edge between them makes it worth stopping for.
func (l *Line) walkPipe(cmd *syntax.BinaryCmd) {
	before := len(l.Commands)
	l.walk(cmd.X)
	mid := len(l.Commands)
	l.walk(cmd.Y)

	// Everything produced by the left side feeds the first command on the
	// right; for `a | b | c` the recursion links a->b and then b->c.
	if mid > before && mid < len(l.Commands) {
		l.Commands[mid-1].PipedInto = append(l.Commands[mid-1].PipedInto, l.Commands[mid].Name)
	}
}

// newCommand converts one call expression, looking through wrappers.
func newCommand(call *syntax.CallExpr) *Command {
	words := make([]string, 0, len(call.Args))
	for _, w := range call.Args {
		words = append(words, wordText(w))
	}
	if len(words) == 0 {
		return nil
	}

	c := &Command{Flags: map[string]bool{}}

	// Strip privilege and environment wrappers, along with their own flags, so
	// the command that actually runs is the one judged.
	for len(words) > 0 {
		name := base(words[0])
		valueFlags, isWrapper := wrappers[name]
		if !isWrapper {
			break
		}

		c.Sudo = c.Sudo || name == "sudo" || name == "doas"
		words = words[1:]

		for len(words) > 0 && strings.HasPrefix(words[0], "-") && words[0] != "-" && words[0] != "--" {
			flags := parseFlag(words[0])
			// A flag written as --user=root carries its value already.
			takesValue := !strings.Contains(words[0], "=") &&
				len(flags) > 0 && valueFlags[flags[len(flags)-1]]
			words = words[1:]
			if takesValue && len(words) > 0 {
				words = words[1:]
			}
		}

		// `env FOO=bar cmd` -- skip the assignments too.
		for len(words) > 0 && isAssignment(words[0]) {
			words = words[1:]
		}
	}
	if len(words) == 0 {
		return nil
	}

	c.Name = base(words[0])
	c.Args = words[1:]
	for _, a := range c.Args {
		if flags := parseFlag(a); len(flags) > 0 {
			for _, f := range flags {
				c.Flags[f] = true
			}
			continue
		}
		c.Positional = append(c.Positional, a)
	}
	return c
}

// parseFlag splits an argument into the flag names it carries, or returns nil
// if it is not a flag. "-rf" yields r and f; "--namespace=x" yields namespace.
func parseFlag(arg string) []string {
	switch {
	case arg == "-" || arg == "--" || !strings.HasPrefix(arg, "-"):
		return nil

	case strings.HasPrefix(arg, "--"):
		name, _, _ := strings.Cut(arg[2:], "=")
		if name == "" {
			return nil
		}
		return []string{name}

	default:
		body, _, _ := strings.Cut(arg[1:], "=")

		// A bundle of short flags; a value may be attached, as in -n5.
		var out []string
		for _, r := range body {
			if r >= '0' && r <= '9' {
				break
			}
			out = append(out, string(r))
		}

		// Plenty of tools spell long options with a single dash -- find's
		// -delete, java's -version, go's -race. Record the whole token as a
		// name too, so a rule can ask for "delete" and mean it. Reading -rf as
		// both {r, f} and "rf" costs nothing: no rule asks for "rf".
		if len(body) > 1 && isFlagName(body) {
			out = append(out, body)
		}
		return out
	}
}

// wordText renders a word as the text it stands for, so that quoting does not
// hide a match: "rm", 'rm' and rm all read as rm.
func wordText(w *syntax.Word) string {
	var b strings.Builder
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			b.WriteString(p.Value)
		case *syntax.SglQuoted:
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, inner := range p.Parts {
				if lit, ok := inner.(*syntax.Lit); ok {
					b.WriteString(lit.Value)
				}
			}
		case *syntax.ParamExp:
			// Keep the parameter visible, so rules can still see $HOME.
			b.WriteString("$" + p.Param.Value)
		}
	}
	return b.String()
}

// base strips a leading path from an executable, so /bin/rm reads as rm.
func base(name string) string {
	if i := strings.LastIndex(name, "/"); i >= 0 {
		return name[i+1:]
	}
	return name
}

// isFlagName reports whether s could be a long option's name.
func isFlagName(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// isAssignment reports whether a word looks like NAME=value.
func isAssignment(w string) bool {
	name, _, found := strings.Cut(w, "=")
	return found && name != "" && !strings.HasPrefix(w, "-")
}
