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
		Config:      config.Config{Provider: "openai", Model: config.DefaultModel},
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
		Config: config.Config{Provider: "openai"},
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
		Config: config.Config{Provider: "openai", Model: config.DefaultModel},
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
