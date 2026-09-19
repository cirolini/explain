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

`explain` talks to OpenAI by default, using the model in
[`internal/config/config.go`](internal/config/config.go). Every provider and
model default lives in that one file.

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
flags. `--model` overrides the configured model for one run.

Point `base_url` at any OpenAI-compatible server — Ollama, vLLM, LM Studio — to
run against a local model. Such servers usually need no API key.

```toml
provider = "openai-compatible"
base_url = "http://localhost:11434/v1"
model    = "llama3"
```

## Docker

```bash
docker build -t explain .
docker run --rm -e EXPLAIN_API_KEY="sk-..." explain "ls -lrth"
```

## Privacy

The command text is sent to whichever provider you configure. If that matters
for what you are about to run, use `base_url` with a local model — nothing
leaves the machine.

## Credits

Originally built in 2023 by [Rafael Cirolini](https://github.com/cirolini) and
[Ádran Farias Carnavale](https://github.com/crnvl96).
