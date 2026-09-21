# explain as a Claude Code hook

A `PreToolUse` hook that runs `explain check` against every Bash command
Claude Code is about to execute.

| Verdict | Decision | What you see |
| --- | --- | --- |
| HIGH | `deny` | The command is blocked, with the rule's reason. |
| MEDIUM | `ask` | You are asked to approve it. |
| LOW | none | Your normal permission settings apply. |

## Why LOW returns no decision

A hook returning `"allow"` grants permission the user never granted — it
overrides your own settings. So on LOW this hook stays quiet and lets Claude
Code's normal permission flow run, which is the behaviour you already had.

The same reasoning covers failure. If `explain` is missing or broken, the hook
emits no decision rather than a verdict it does not have. That is not the same
as allowing: you fall back to your existing permission settings, so a broken
hook leaves you where you were instead of opening a hole or blocking all work.

## Install

Requires `explain` and `jq` on `PATH`.

Copy [`explain-hook.sh`](explain-hook.sh) somewhere stable, make it
executable, and add the hook to `.claude/settings.json` (see
[`settings.json`](settings.json) for the shape):

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          { "type": "command", "command": "/path/to/explain-hook.sh" }
        ]
      }
    ]
  }
}
```

## Cost

None. `explain check` consults no model by default: the verdict comes from
rules that need no network and answer in well under a millisecond. That is
deliberate — a guardrail costing an API round trip per command is a guardrail
someone switches off, and one that fails when the network does is worse than
none.

Add `--explain` to the hook command if you want a model's description in the
denial reason, and accept the latency and cost that come with it.

## Try it

```console
$ echo '{"tool_name":"Bash","tool_input":{"command":"rm -rf /"}}' | ./explain-hook.sh
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "deny",
    "permissionDecisionReason": "explain: Recursively force-deletes a top-level or home path. There is no confirmation and no undo. (+1 more)"
  }
}
```

## Limits

The rules are a floor, not a proof. They catch shapes known to be destructive;
they do not understand what your particular command will do to your particular
machine. A LOW verdict means no rule matched, not that the command is safe.
