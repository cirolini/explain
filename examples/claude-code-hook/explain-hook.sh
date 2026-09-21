#!/usr/bin/env bash
#
# A PreToolUse hook for Claude Code that runs `explain check` against every
# Bash command before it executes.
#
#   HIGH    -> deny, with the rule's reason
#   MEDIUM  -> ask the user
#   LOW     -> no decision; your normal permission settings apply
#
# Requires: explain, jq.
#
# See README.md in this directory for installation.

set -uo pipefail

input=$(cat)

# Only Bash commands are ours to judge.
tool=$(jq -r '.tool_name // empty' <<<"$input")
[[ "$tool" == "Bash" ]] || exit 0

command=$(jq -r '.tool_input.command // empty' <<<"$input")
[[ -n "$command" ]] || exit 0

# `explain check` exits 0/1/2 by risk level, so a non-zero status here is the
# normal path, not a failure. Anything at 3 or above means explain itself
# could not answer.
verdict=$(explain check --json -- "$command" 2>/dev/null)
status=$?

decide() {
  jq -n --arg decision "$1" --arg reason "$2" '{
    hookSpecificOutput: {
      hookEventName: "PreToolUse",
      permissionDecision: $decision,
      permissionDecisionReason: $reason
    }
  }'
}

# The findings come back most severe first, so the first one is the reason the
# verdict is what it is. A permission dialog has one line of attention; the
# rest are counted rather than printed.
reason() {
  jq -r '
    (.findings // []) as $f
    | if ($f | length) == 0 then "flagged by explain"
      else $f[0].reason + (if ($f | length) > 1 then " (+\($f | length - 1) more)" else "" end)
      end
  ' <<<"$verdict" 2>/dev/null || echo "flagged by explain"
}

case "$status" in
  2) decide deny "explain: $(reason)" ;;
  1) decide ask  "explain: $(reason)" ;;
  0) exit 0 ;;
  *)
    # explain could not answer -- a missing binary, a broken config. Emit no
    # decision rather than a verdict we do not have. That is not the same as
    # allowing: with no decision, Claude Code falls back to the permission
    # settings you already had, so a broken hook leaves you where you were
    # rather than opening a hole or blocking all work.
    exit 0
    ;;
esac
