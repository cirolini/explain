# Examples

Two ways to put `explain` in front of a command before it runs.

| | |
| --- | --- |
| [`claude-code-hook/`](claude-code-hook/) | A `PreToolUse` hook that checks every command a coding agent wants to run. |
| [`shell/`](shell/) | A `??` prefix for your own shell. |

Both use `explain check`, which classifies by rules alone — no model, no
network, no API key — unless you pass `--explain`.
