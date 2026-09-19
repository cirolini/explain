package cli

import (
	"bytes"
	"encoding/json"
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

// The verdict must be printed before the model is asked anything: it comes
// from rules, so it exists even when the model is slow, wrong, or absent.
func TestVerdictIsPrintedBeforeTheExplanation(t *testing.T) {
	fake := &llm.Fake{Response: "SEVERITY: HIGH\n\nDeletes everything."}

	out, err := run(t, fake, "rm", "-rf", "/")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	verdictAt := strings.Index(out, "HIGH —")
	prose := strings.Index(out, "Deletes everything.")
	if verdictAt < 0 {
		t.Fatalf("no verdict line in output:\n%s", out)
	}
	if prose >= 0 && verdictAt > prose {
		t.Errorf("verdict came after the explanation:\n%s", out)
	}
}

func TestVerdictComesFromRulesNotTheModel(t *testing.T) {
	// The model insists the command is harmless. The rules disagree, and the
	// rules are what the user sees.
	fake := &llm.Fake{Response: "SEVERITY: LOW\n\nThis is a routine cleanup command."}

	out, err := run(t, fake, "rm", "-rf", "/")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "HIGH") {
		t.Errorf("verdict was talked down by the model:\n%s", out)
	}
}

// explain's own marker is machinery, not output.
func TestSeverityMarkerIsNotShownToTheUser(t *testing.T) {
	fake := &llm.Fake{Response: "SEVERITY: HIGH\n\nDeletes everything."}

	out, err := run(t, fake, "rm", "-rf", "/")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Contains(out, "SEVERITY:") {
		t.Errorf("the severity marker leaked into the output:\n%s", out)
	}
}

// A model that rates a command higher than the rules did can raise it.
func TestModelCanRaiseTheVerdict(t *testing.T) {
	fake := &llm.Fake{Response: "SEVERITY: HIGH\n\nThis pipes a remote script into a shell."}

	out, err := run(t, fake, "somecmd", "--obscure-flag")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Raised to HIGH") {
		t.Errorf("no escalation note when the model raised the verdict:\n%s", out)
	}
}

func TestJSONOutputIsValidAndRuleLed(t *testing.T) {
	fake := &llm.Fake{Response: "SEVERITY: LOW\n\nRoutine cleanup."}

	var out bytes.Buffer
	cmd := NewCommand(Options{
		Config:      config.Config{Provider: config.ProviderOpenAI},
		NewProvider: func(config.Config) (llm.Provider, error) { return fake, nil },
		Out:         &out,
	})
	cmd.SetArgs([]string{"--json", "rm", "-rf", "/"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var got struct {
		Command     string `json:"command"`
		Severity    string `json:"severity"`
		Explanation string `json:"explanation"`
		Findings    []struct {
			Rule string `json:"rule"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out.String())
	}

	if got.Severity != "HIGH" {
		t.Errorf("severity = %q, want HIGH despite the model saying LOW", got.Severity)
	}
	if got.Command != "rm -rf /" {
		t.Errorf("command = %q, want the command as given", got.Command)
	}
	if strings.Contains(got.Explanation, "SEVERITY:") {
		t.Errorf("the marker leaked into the JSON explanation: %q", got.Explanation)
	}
	if len(got.Findings) == 0 {
		t.Error("no findings in JSON output")
	}
}

// JSON must be one clean object, never a stream with prose in front of it.
func TestJSONOutputHasNothingBeforeIt(t *testing.T) {
	fake := &llm.Fake{Response: "SEVERITY: HIGH\n\nDeletes everything."}

	var out bytes.Buffer
	cmd := NewCommand(Options{
		Config:      config.Config{Provider: config.ProviderOpenAI},
		NewProvider: func(config.Config) (llm.Provider, error) { return fake, nil },
		Out:         &out,
	})
	cmd.SetArgs([]string{"--json", "rm", "-rf", "/"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if first := strings.TrimSpace(out.String())[0]; first != '{' {
		t.Errorf("output does not begin with the JSON object:\n%s", out.String())
	}
}

// The rule findings are given to the model so the explanation addresses them.
func TestRuleFindingsReachThePrompt(t *testing.T) {
	fake := &llm.Fake{Response: "ok"}

	if _, err := run(t, fake, "rm", "-rf", "/"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(fake.Last.User, "force-deletes") {
		t.Errorf("rule reasons missing from the prompt:\n%s", fake.Last.User)
	}
}
