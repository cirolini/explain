package risk

import "slices"

// matches reports whether the rule fires on the given line.
func (r *Rule) matches(line Line) bool {
	// A text-only rule reads the raw line, so it still works when the shell
	// parser could not. This is what keeps a malformed or exotic line from
	// silently coming back LOW.
	if r.Match.isTextOnly() {
		return r.Match.matchesText(line.Raw)
	}
	if !line.Parsed {
		return false
	}

	for _, c := range line.Commands {
		if r.Match.matchesCommand(c, line.Raw) {
			return true
		}
	}
	return false
}

// matchesText reports whether any text pattern matches the raw line.
func (m Match) matchesText(raw string) bool {
	for _, re := range m.textRe {
		if re.MatchString(raw) {
			return true
		}
	}
	return false
}

// matchesCommand reports whether every condition set on the match holds for
// this command. An unset condition is not a wildcard that matches everything;
// it is simply not checked.
func (m Match) matchesCommand(c *Command, raw string) bool {
	if len(m.Command) > 0 && !slices.Contains(m.Command, c.Name) {
		return false
	}

	// The subcommand is the first positional argument: `git push`, not a flag.
	if len(m.Subcommand) > 0 {
		if len(c.Positional) == 0 || !slices.Contains(m.Subcommand, c.Positional[0]) {
			return false
		}
	}

	// Flags: all must be present.
	for _, f := range m.Flags {
		if !c.HasFlag(f) {
			return false
		}
	}

	// AnyFlag: at least one.
	if len(m.AnyFlag) > 0 && !slices.ContainsFunc(m.AnyFlag, c.HasFlag) {
		return false
	}

	// MissingFlags: none may be present. This is how a rule fires on an
	// omission rather than on something written.
	if slices.ContainsFunc(m.MissingFlags, c.HasFlag) {
		return false
	}

	if len(m.argRe) > 0 && !m.matchesArg(c) {
		return false
	}

	if len(m.PipeInto) > 0 {
		if !slices.ContainsFunc(c.PipedInto, func(d string) bool {
			return slices.Contains(m.PipeInto, d)
		}) {
			return false
		}
	}

	if len(m.textRe) > 0 && !m.matchesText(raw) {
		return false
	}

	return true
}

// matchesArg reports whether any positional argument matches any pattern.
func (m Match) matchesArg(c *Command) bool {
	for _, arg := range c.Positional {
		for _, re := range m.argRe {
			if re.MatchString(arg) {
				return true
			}
		}
	}
	return false
}
