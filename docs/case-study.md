# From "explain this command" to "should this run?"

> **SCAFFOLD — NOT YET WRITTEN.**
>
> Claude assembled the structure and the verified facts. The prose is Rafael's
> to write: this is his argument, in his voice, in the first person.
>
> Every `**Write:**` block below is a prompt, not a draft. Every fact under
> them was checked against the repository, the git history, or a command that
> was actually run — dates and numbers are safe to quote. Delete this callout
> and all the `**Write:**` markers when the prose is in.

---

## 1. What it was

> **Write:** what you and Ádran built in 2023, and why. One or two paragraphs.
> The honest version: it was a convenience for humans, not a safety tool.

Facts:

- First release `1.0.0`; last commit February 2023; 18 stars.
- Go 1.19, `sashabaranov/go-gpt3 v1.1.0`, the legacy Completions API.
- One argument in, an explanation out. No risk judgement of any kind.
- Co-written with [Ádran Farias Carnavale](https://github.com/crnvl96).

## 2. What broke, and what that taught me

> **Write:** the moment you went back to it. Whether you knew it was broken.
> The point worth landing: it did not break because the code was wrong. It
> broke because a constant in it referred to something outside it that stopped
> existing.

Facts:

- `internal/GPTClient/GenerateRequest.go` hardcoded `text-davinci-003`.
- OpenAI deprecated that model on 2023-07-06 and **shut it down on
  2024-01-04**. Every run after that returned `model_not_found`.
- The legacy `/v1/completions` endpoint is *not* on OpenAI's deprecation list.
  The model went; the endpoint stayed. The tool died of a constant.
- `sashabaranov/go-gpt3` was renamed to `go-openai`. The pinned import path
  still resolves through the module proxy, but nothing has been published
  under it since.
- Three bugs that were never about the API, found by reading and confirmed by
  running the 1.x binary:
  - `RetrieveAIAPIKeyFromFile` called `os.Exit(1)` when `~/.explainrc` was
    missing, so the documented `API_KEY` fallback was unreachable — including
    in the Docker image, which the README documented as env-only. That path
    never worked.
  - `MaxTokens: 4000` against davinci's 4097-token context left ~97 tokens for
    the prompt.
  - `script.sh` was `./explain $PROMPT`, unquoted, word-splitting exactly the
    commands worth explaining.

> **Write:** the lesson you actually draw from this. A candidate, take it or
> leave it: a tool that depends on a vendor's moving parts needs those parts
> in one place you can change, not scattered through the code. In v2 every
> model name lives in `internal/config`.

## 3. What changed underneath it

> **Write:** short. This is not the interesting part and should not read as
> if it were.

Facts:

- Go 1.26, module renamed to `github.com/cirolini/explain` so `go install`
  works.
- Official `openai-go/v3` and `anthropic-sdk-go`, chosen over the third-party
  wrapper deliberately: v1 died because a wrapper pinned a constant.
- Chat Completions rather than the newer Responses API, because
  OpenAI-compatible servers — Ollama, vLLM, LM Studio — implement Chat
  Completions. One code path reaches OpenAI, Claude and a model on your own
  machine.
- 113 tests where there were none.

## 4. Why "explain" became "check"

> **Write:** the heart of it. What changed between 2023 and now is not the
> tool, it is who runs the commands. Coding agents run shell commands on your
> machine continuously, and nobody reads them all. "Understand this before you
> run it" was a convenience when a human typed it. It is a guardrail when an
> agent generates it.

Facts:

- `explain check "<cmd>"` exits `0` LOW, `1` MEDIUM, `2` HIGH, `3` explain
  itself failed.
- A verdict and a failure are different exit codes on purpose. A caller that
  cannot tell "this command is dangerous" from "explain could not run" has to
  either block on outages or ignore real findings.
- `check` consults no model by default. The rules need no network, no API key
  and no credit.

> **Write:** why the default matters. The argument: a guardrail that costs an
> API round trip per command is one that gets switched off, and one that fails
> when the network does is worse than none.

## 5. The split: rules decide, the model describes

> **Write:** the design claim, in your words. This is the section people will
> quote.

Facts:

- 30 rules in `internal/risk/rules.yaml`, as data: id, severity, reason, match.
- The verdict comes from rules alone. A model may **raise** a verdict. It can
  never lower one.
- Matching is shell-aware, not regex over the raw line — the line is parsed
  with `mvdan.cc/sh`. That is what lets `curl x | sh` be caught while
  `curl x | jq .` is not: for that rule, the *edge between two commands* is the
  entire finding, since neither command is alarming alone.
- Consequences that fall out of parsing properly: `sudo rm -rf /var` is judged
  as `rm`; quoting cannot hide a command; `$HOME` stays visible; a rule can
  fire on an *omission*, like `kubectl delete` with no `-n`.
- 71 cases in `internal/risk/testdata/commands.yaml` pin the rules. No model,
  no network — exact.
- A separate test fails if any rule is never exercised by that table. It
  caught two dead rules on its first run.

## 6. The command is not a trusted input

> **Write:** this is the section that justifies the whole split. A command can
> contain text aimed at the model rather than the shell. If the model decides
> the verdict, the command can argue with the verdict.

Facts:

- The command reaches the model fenced inside a delimiter carrying a per-run
  random nonce, and the system prompt states the command is data, never
  instructions.
- That is mitigation, not a guarantee — which is the reason the verdict does
  not depend on it.
- Measured, not asserted: the eval was run twice more against stubs.
  - A model answering `SEVERITY: LOW` to everything — what a successful
    injection produces — gave results **identical** to rules-only across all
    66 cases.
  - A model answering `SEVERITY: HIGH` to everything raised verdicts, and
    understated none.
- A line the shell parser cannot read never returns LOW. It floors at MEDIUM
  and says the assessment is partial. Otherwise the bypass writes itself:
  craft something this parser rejects but a shell accepts, and a hook that
  trusts LOW waves it through.

## 7. In front of an agent

> **Write:** what it is like to actually use, and whether you kept it on.

Facts:

- `examples/claude-code-hook/` is a `PreToolUse` hook: HIGH denies, MEDIUM
  asks, LOW emits **no decision at all**.
- LOW returning no decision rather than `"allow"` is deliberate: a hook that
  returns `allow` grants permission the user never granted, overriding their
  own settings.
- The same holds on failure. A missing or broken `explain` emits no decision,
  so you fall back to the permission settings you already had — not to an open
  door, and not to a blocked agent.
- `examples/shell/` adds a `??` prefix for a human's own shell.

## 8. What the numbers say

> **Write:** your reading of the results. Be careful here — see §9.

Facts, from [`results.md`](results.md):

- 66 commands, 44 of them agent-typical. Deliberately **not** drawn from the
  rules test table: a dataset built from the rules scores perfectly by
  construction and measures nothing.
- Rules only: **77%** agreement before the last round of rules, **86%** after.
- **Zero false positives in both.** `curl … -o install.sh`,
  `git push origin main`, `dd if=/dev/sda of=backup.img`, `npm install`,
  `terraform plan`, `kubectl get pods` all return LOW.
- The eval surfaced 15 commands rated below their label. Six were unambiguous
  misses and got rules. The other nine were left alone as judgement calls.

> **Write:** why zero false positives is the number you care about most, if
> you agree that it is. The argument available: a guardrail that nags gets
> switched off, and a switched-off guardrail catches nothing.

## 9. What I do not know

> **Write:** in your own words, and do not soften it. This section is why the
> rest is worth believing.

Facts:

- **The labels are draft and Claude wrote them.** Whoever tunes the rules must
  not also set the targets, or the numbers measure self-consistency rather
  than quality. Until you have reviewed them, 86% is a starting point and not
  a measurement.
- **The model arm has never run against a real model.** There was no funded
  key available during the build. The arm is implemented and exercised against
  stubs; no figures from a real model exist yet.
- Nine known misses are listed in `results.md`, unfixed on purpose.
- The rules are a floor, not a proof. **A LOW verdict means no rule matched —
  not that the command is safe.**
- The rules encode one person's judgement about a few dozen command shapes.
  They do not know your machine, your data, or what that path means to you.

## 10. What I would tell someone building the same thing

> **Write:** the takeaway. Whatever you actually believe. Some candidates,
> pick or discard:
>
> - Put the part that decides somewhere you can read, test and diff. Put the
>   part that explains behind a model.
> - Anything a model reads from an untrusted source is an input, not an
>   instruction — and if a model decides, whatever it reads can argue with the
>   decision.
> - A dataset drawn from your own implementation measures nothing.
> - Zero false positives buys more safety than a higher hit rate, because the
>   failure mode of a noisy guardrail is that it gets removed.
> - Failing to "no decision" is not the same as failing to "allow".

---

> **Write:** a closing line, and credit Ádran here as well as in the README.
