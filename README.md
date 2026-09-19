# explain

`explain` describes what a shell command does — what it touches, and whether
its effects can be undone — without running it.

```console
$ explain "ls -lrth"
$ explain tar -xzf archive.tar.gz -C /opt
```

It never executes the command it is given.

## Install

Build from source. `explain` needs Go 1.26 or newer.

```bash
go install github.com/cirolini/explain/cmd/explain@latest
```

## Configure

`explain` talks to OpenAI by default. Every provider and model default lives in
[`internal/config/config.go`](internal/config/config.go) — one file to edit when
a provider retires a model.

Create `~/.config/explain/config.toml`:

```toml
provider = "openai"
model    = "gpt-5.6-luna"
api_key  = "sk-..."
```

Or set the environment instead:

```bash
export EXPLAIN_API_KEY="sk-..."   # OPENAI_API_KEY and API_KEY also work
export EXPLAIN_MODEL="gpt-5.6-terra"
```

Settings are resolved lowest to highest: built-in defaults, `config.toml`,
`~/.explainrc` (the 1.x key file, still read), environment variables, then
flags.

## Providers

| Flag | Provider | Default model |
| --- | --- | --- |
| `--provider openai` | OpenAI | `gpt-5.6-luna` |
| `--provider anthropic` | Claude API | `claude-opus-5` |
| `--provider openai-compatible` | Any OpenAI-compatible endpoint | none — pass `--model` |

Choosing a provider picks up that provider's own default model, so
`--provider anthropic` does not try to send an OpenAI model name.

### Run it locally

Point `--base-url` at any OpenAI-compatible server — Ollama, vLLM, LM Studio —
and nothing leaves the machine. Such servers usually need no API key.

```bash
explain --base-url http://localhost:11434/v1 --model llama3 "ls -lrth"
```

Passing `--base-url` alone is enough; the provider switches to
`openai-compatible` on its own.

## Flags

```
--provider   openai, anthropic or openai-compatible
--model      model to use (default: the provider's own)
--base-url   OpenAI-compatible endpoint
--lang       explanation language: en or pt
```

Explanations stream as they arrive, so the terminal starts filling immediately.

`--lang pt` writes the explanation in Brazilian Portuguese. Command names,
flags and paths are left exactly as they are — only the prose is translated.

## Docker

```bash
docker build -t explain .
docker run --rm -e EXPLAIN_API_KEY="sk-..." explain "ls -lrth"
```

## Privacy

The command text is sent to whichever provider you configure. If that matters
for what you are about to run, use `--base-url` with a local model — nothing
leaves the machine.

The command is treated as untrusted input to the model: it is fenced inside a
delimiter carrying a per-run random nonce, and the system prompt states that
the command is data to describe, never instructions to follow. That is
mitigation, not a guarantee.

## Credits

Originally built in 2023 by [Rafael Cirolini](https://github.com/cirolini) and
[Ádran Farias Carnavale](https://github.com/crnvl96).
