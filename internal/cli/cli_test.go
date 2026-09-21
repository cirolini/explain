package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/cirolini/explain/internal/config"
	"github.com/cirolini/explain/internal/llm"
)

// run executes the command with the given args against a fake provider and
// returns what it printed alongside any error.
func run(t *testing.T, fake *llm.Fake, args ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	cmd := NewCommand(Options{
		Config:      config.Config{Provider: config.ProviderOpenAI},
		NewProvider: func(config.Config) (llm.Provider, error) { return fake, nil },
		Out:         &out,
	})
	cmd.SetArgs(args)
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	// Execute before reading the buffer: Go evaluates return expressions left
	// to right, so returning out.String() alongside cmd.Execute() would snapshot
	// the buffer before the command ever wrote to it.
	err := cmd.Execute()
	return out.String(), err
}

func TestRunPrintsExplanation(t *testing.T) {
	fake := &llm.Fake{Response: "lists files by modification time"}

	out, err := run(t, fake, "ls -lrth")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "lists files by modification time") {
		t.Errorf("output = %q, want the explanation", out)
	}
	if fake.Calls != 1 {
		t.Errorf("provider called %d times, want 1", fake.Calls)
	}
}

func TestRunPassesTheCommandToTheProvider(t *testing.T) {
	fake := &llm.Fake{Response: "ok"}

	if _, err := run(t, fake, "ls -lrth"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(fake.Last.User, "ls -lrth") {
		t.Errorf("prompt does not contain the command:\n%s", fake.Last.User)
	}
}

// `explain ls -lrth` should work as well as `explain "ls -lrth"`.
func TestRunJoinsTrailingArguments(t *testing.T) {
	fake := &llm.Fake{Response: "ok"}

	if _, err := run(t, fake, "ls", "-lrth", "/tmp"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(fake.Last.User, "ls -lrth /tmp") {
		t.Errorf("arguments were not joined:\n%s", fake.Last.User)
	}
}

// Flags belonging to the command being explained must not be eaten by cobra.
func TestRunDoesNotInterpretTheCommandsOwnFlags(t *testing.T) {
	fake := &llm.Fake{Response: "ok"}

	if _, err := run(t, fake, "rm", "-rf", "/tmp/build"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(fake.Last.User, "rm -rf /tmp/build") {
		t.Errorf("command was mangled:\n%s", fake.Last.User)
	}
}

func TestRunRequiresACommand(t *testing.T) {
	if _, err := run(t, &llm.Fake{}); err == nil {
		t.Fatal("Execute succeeded with no arguments, want error")
	}
}

// Provider failures must come back as errors for main to report and exit on --
// not as an os.Exit from inside the call, which is what 1.x did and what made
// the code untestable.
func TestRunReturnsProviderErrors(t *testing.T) {
	want := errors.New("model_not_found")

	_, err := run(t, &llm.Fake{Err: want})
	if err == nil {
		t.Fatal("Execute succeeded despite a provider error")
	}
}

func TestRunReturnsProviderConstructionErrors(t *testing.T) {
	var out bytes.Buffer
	cmd := NewCommand(Options{
		Config: config.Config{Provider: config.ProviderOpenAI},
		NewProvider: func(config.Config) (llm.Provider, error) {
			return nil, config.ErrNoAPIKey
		},
		Out: &out,
	})
	cmd.SetArgs([]string{"ls"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); !errors.Is(err, config.ErrNoAPIKey) {
		t.Errorf("err = %v, want ErrNoAPIKey", err)
	}
}

// --model overrides the configured default, so a retired default never leaves
// a user stuck the way the hardcoded model in 1.x did.
func TestModelFlagOverridesConfig(t *testing.T) {
	var got config.Config
	var out bytes.Buffer

	cmd := NewCommand(Options{
		Config: config.Config{Provider: config.ProviderOpenAI},
		NewProvider: func(cfg config.Config) (llm.Provider, error) {
			got = cfg
			return &llm.Fake{Response: "ok"}, nil
		},
		Out: &out,
	})
	cmd.SetArgs([]string{"--model", "gpt-5.6-terra", "ls"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.Model != "gpt-5.6-terra" {
		t.Errorf("Model = %q, want %q", got.Model, "gpt-5.6-terra")
	}
}

// execute runs the command with a provider factory that captures the resolved
// configuration, so flag handling can be asserted without a network call.
func execute(t *testing.T, cfg config.Config, args ...string) (config.Config, error) {
	t.Helper()

	var got config.Config
	var out bytes.Buffer
	cmd := NewCommand(Options{
		Config: cfg,
		NewProvider: func(c config.Config) (llm.Provider, error) {
			got = c
			return &llm.Fake{Response: "ok"}, nil
		},
		Out: &out,
	})
	cmd.SetArgs(args)
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	return got, cmd.Execute()
}

func TestProviderFlagSelectsTheProvider(t *testing.T) {
	got, err := execute(t, config.Config{Provider: config.ProviderOpenAI},
		"--provider", config.ProviderAnthropic, "ls")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.Provider != config.ProviderAnthropic {
		t.Errorf("Provider = %q, want %q", got.Provider, config.ProviderAnthropic)
	}
}

// Choosing a provider with --provider must pick up that provider's default
// model, not the one the previous provider would have used.
func TestProviderFlagPicksUpThatProvidersDefaultModel(t *testing.T) {
	got, err := execute(t, config.Config{Provider: config.ProviderOpenAI},
		"--provider", config.ProviderAnthropic, "ls")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if want := config.DefaultModels[config.ProviderAnthropic]; got.Model != want {
		t.Errorf("Model = %q, want %q", got.Model, want)
	}
}

func TestBaseURLFlagSwitchesToTheCompatibleProvider(t *testing.T) {
	got, err := execute(t, config.Config{Provider: config.ProviderOpenAI},
		"--base-url", "http://localhost:11434/v1", "--model", "llama3", "ls")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.Provider != config.ProviderCompatible {
		t.Errorf("Provider = %q, want %q", got.Provider, config.ProviderCompatible)
	}
	if got.BaseURL != "http://localhost:11434/v1" {
		t.Errorf("BaseURL = %q, want the flag value", got.BaseURL)
	}
}

func TestLangFlagReachesThePrompt(t *testing.T) {
	fake := &llm.Fake{Response: "ok"}
	var out bytes.Buffer
	cmd := NewCommand(Options{
		Config:      config.Config{Provider: config.ProviderOpenAI},
		NewProvider: func(config.Config) (llm.Provider, error) { return fake, nil },
		Out:         &out,
	})
	cmd.SetArgs([]string{"--lang", "pt", "ls"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(fake.Last.System, "Brazilian Portuguese") {
		t.Errorf("system prompt is not in pt mode:\n%s", fake.Last.System)
	}
}

func TestInvalidFlagValuesAreRejected(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"unknown provider", []string{"--provider", "gopher", "ls"}},
		{"unknown language", []string{"--lang", "klingon", "ls"}},
		{"compatible without base url", []string{"--provider", "openai-compatible", "ls"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := execute(t, config.Config{Provider: config.ProviderOpenAI}, tc.args...); err == nil {
				t.Fatal("Execute succeeded, want error")
			}
		})
	}
}

// Streaming means the explanation reaches the terminal as it arrives.
func TestRunStreamsOutput(t *testing.T) {
	fake := &llm.Fake{Response: "lists files"}

	out, err := run(t, fake, "ls")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "lists files") {
		t.Errorf("output = %q, want the streamed explanation", out)
	}
}
