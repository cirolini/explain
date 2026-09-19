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
	if cfg.Model != DefaultModel {
		t.Errorf("Model = %q, want %q", cfg.Model, DefaultModel)
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
