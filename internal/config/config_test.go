package config

import (
	"os"
	"path/filepath"
	"testing"
)

// env builds a Getenv stand-in from a map.
func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

// writeFile creates a file and every directory leading to it.
func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Loader{HomeDir: t.TempDir(), Getenv: env(nil)}.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Provider != DefaultProvider {
		t.Errorf("Provider = %q, want %q", cfg.Provider, DefaultProvider)
	}
	// Model stays empty until Resolve runs, so that a provider chosen by a
	// flag picks up its own default rather than the previous provider's.
	if cfg.Model != "" {
		t.Errorf("Model = %q, want empty before Resolve", cfg.Model)
	}
	if cfg.Lang != DefaultLang {
		t.Errorf("Lang = %q, want %q", cfg.Lang, DefaultLang)
	}
	if cfg.APIKey != "" {
		t.Errorf("APIKey = %q, want empty", cfg.APIKey)
	}
}

func TestLoadConfigFile(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".config", "explain", "config.toml"), `
provider = "openai-compatible"
model = "llama3"
base_url = "http://localhost:11434/v1"
api_key = "from-file"
`)

	cfg, err := Loader{HomeDir: home, Getenv: env(nil)}.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, tc := range []struct{ name, got, want string }{
		{"Provider", cfg.Provider, "openai-compatible"},
		{"Model", cfg.Model, "llama3"},
		{"BaseURL", cfg.BaseURL, "http://localhost:11434/v1"},
		{"APIKey", cfg.APIKey, "from-file"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

func TestLoadMalformedConfigFileIsAnError(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".config", "explain", "config.toml"), "model = \n")

	if _, err := (Loader{HomeDir: home, Getenv: env(nil)}).Load(); err == nil {
		t.Fatal("Load succeeded on malformed TOML, want error")
	}
}

func TestLoaderRespectsXDGConfigHome(t *testing.T) {
	home, xdg := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(xdg, "explain", "config.toml"), `model = "from-xdg"`)

	cfg, err := Loader{HomeDir: home, ConfigDir: xdg, Getenv: env(nil)}.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Model != "from-xdg" {
		t.Errorf("Model = %q, want %q", cfg.Model, "from-xdg")
	}
}

// Regression test for the bug that made explain 1.x unusable without the key
// file: RetrieveAIAPIKeyFromFile called os.Exit(1) when ~/.explainrc was
// missing, so the documented API_KEY fallback could never be reached.
func TestLoadFallsBackToEnvWhenLegacyFileMissing(t *testing.T) {
	cfg, err := Loader{
		HomeDir: t.TempDir(),
		Getenv:  env(map[string]string{"API_KEY": "sk-from-env"}),
	}.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIKey != "sk-from-env" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "sk-from-env")
	}
}

func TestLoadLegacyKeyFile(t *testing.T) {
	home := t.TempDir()
	// 1.x documented `echo {KEY} > ~/.explainrc`, which leaves a trailing
	// newline; a hand-edited file may leave surrounding whitespace too.
	writeFile(t, filepath.Join(home, ".explainrc"), "  sk-legacy-key \n")

	cfg, err := Loader{HomeDir: home, Getenv: env(nil)}.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIKey != "sk-legacy-key" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "sk-legacy-key")
	}
}

func TestPrecedence(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".config", "explain", "config.toml"),
		"model = \"from-file\"\napi_key = \"from-file\"\n")
	writeFile(t, filepath.Join(home, ".explainrc"), "from-explainrc\n")

	cfg, err := Loader{
		HomeDir: home,
		Getenv:  env(map[string]string{"EXPLAIN_API_KEY": "from-env"}),
	}.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// env beats .explainrc beats config.toml.
	if cfg.APIKey != "from-env" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "from-env")
	}
	// A source that says nothing about a field leaves it alone.
	if cfg.Model != "from-file" {
		t.Errorf("Model = %q, want %q", cfg.Model, "from-file")
	}
}

func TestAPIKeyEnvPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name string
		vars map[string]string
		want string
	}{
		{"EXPLAIN_API_KEY wins", map[string]string{
			"EXPLAIN_API_KEY": "a", "OPENAI_API_KEY": "b", "API_KEY": "c",
		}, "a"},
		{"OPENAI_API_KEY next", map[string]string{
			"OPENAI_API_KEY": "b", "API_KEY": "c",
		}, "b"},
		{"legacy API_KEY last", map[string]string{"API_KEY": "c"}, "c"},
		{"blank values are skipped", map[string]string{
			"EXPLAIN_API_KEY": "   ", "OPENAI_API_KEY": "b",
		}, "b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Loader{HomeDir: t.TempDir(), Getenv: env(tc.vars)}.Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.APIKey != tc.want {
				t.Errorf("APIKey = %q, want %q", cfg.APIKey, tc.want)
			}
		})
	}
}

func TestResolveFillsTheProviderDefaultModel(t *testing.T) {
	for _, tc := range []struct{ provider, want string }{
		{ProviderOpenAI, DefaultModels[ProviderOpenAI]},
		{ProviderAnthropic, DefaultModels[ProviderAnthropic]},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			cfg := Config{Provider: tc.provider}
			if err := cfg.Resolve(); err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if cfg.Model != tc.want {
				t.Errorf("Model = %q, want %q", cfg.Model, tc.want)
			}
		})
	}
}

// Switching provider must not leave the other provider's model behind -- the
// reason Model is resolved after flags rather than at load time.
func TestResolveDoesNotCarryAModelAcrossProviders(t *testing.T) {
	cfg := Config{Provider: ProviderAnthropic}
	if err := cfg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cfg.Model == DefaultModels[ProviderOpenAI] {
		t.Errorf("anthropic resolved to the OpenAI default %q", cfg.Model)
	}
}

func TestResolveKeepsAnExplicitModel(t *testing.T) {
	cfg := Config{Provider: ProviderAnthropic, Model: "claude-haiku-4-5"}
	if err := cfg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cfg.Model != "claude-haiku-4-5" {
		t.Errorf("Model = %q, want the explicit value", cfg.Model)
	}
}

// A base URL means the endpoint is OpenAI-compatible, not OpenAI itself.
func TestResolveInfersCompatibleProviderFromBaseURL(t *testing.T) {
	cfg := Config{Provider: ProviderOpenAI, BaseURL: "http://localhost:11434/v1", Model: "llama3"}
	if err := cfg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cfg.Provider != ProviderCompatible {
		t.Errorf("Provider = %q, want %q", cfg.Provider, ProviderCompatible)
	}
}

func TestResolveRejectsCompatibleWithoutABaseURL(t *testing.T) {
	cfg := Config{Provider: ProviderCompatible, Model: "llama3"}
	if err := cfg.Resolve(); err == nil {
		t.Fatal("Resolve succeeded without a base URL, want error")
	}
}

// A local server's model names are its own, so there is nothing to guess.
func TestResolveRequiresAModelForCompatible(t *testing.T) {
	cfg := Config{Provider: ProviderCompatible, BaseURL: "http://localhost:11434/v1"}
	if err := cfg.Resolve(); err == nil {
		t.Fatal("Resolve succeeded without a model, want error")
	}
}

func TestResolveRejectsAnUnknownProvider(t *testing.T) {
	cfg := Config{Provider: "gopher"}
	if err := cfg.Resolve(); err == nil {
		t.Fatal("Resolve accepted an unknown provider, want error")
	}
}

func TestResolveLanguage(t *testing.T) {
	for _, tc := range []struct {
		lang    string
		want    string
		wantErr bool
	}{
		{"", DefaultLang, false},
		{LangEN, LangEN, false},
		{LangPT, LangPT, false},
		{"klingon", "", true},
	} {
		t.Run("lang="+tc.lang, func(t *testing.T) {
			cfg := Config{Provider: ProviderOpenAI, Lang: tc.lang}
			err := cfg.Resolve()
			if tc.wantErr {
				if err == nil {
					t.Fatal("Resolve accepted an unknown language, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if cfg.Lang != tc.want {
				t.Errorf("Lang = %q, want %q", cfg.Lang, tc.want)
			}
		})
	}
}

// Every provider with a default model must have an adapter that can be built
// from it -- a default naming a provider explain cannot reach is a dead end.
func TestEveryDefaultModelBelongsToAKnownProvider(t *testing.T) {
	for provider, model := range DefaultModels {
		if model == "" {
			t.Errorf("provider %q has an empty default model", provider)
		}
		cfg := Config{Provider: provider}
		if err := cfg.Resolve(); err != nil {
			t.Errorf("provider %q does not resolve: %v", provider, err)
		}
	}
}
