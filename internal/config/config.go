// Package config resolves explain's settings from the environment, a TOML
// config file, and the legacy ~/.explainrc key file.
//
// Precedence, lowest to highest: built-in defaults, config file, legacy
// .explainrc (API key only), environment variables. Command-line flags are
// applied by the caller on top of the result.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Supported providers.
const (
	// ProviderOpenAI talks to OpenAI directly.
	ProviderOpenAI = "openai"
	// ProviderAnthropic talks to the Claude API.
	ProviderAnthropic = "anthropic"
	// ProviderCompatible talks to any OpenAI-compatible endpoint -- Ollama,
	// vLLM, LM Studio -- and so requires a base URL.
	ProviderCompatible = "openai-compatible"
)

// DefaultProvider is used when none is configured.
const DefaultProvider = ProviderOpenAI

// DefaultModels holds the model each provider uses when none is configured.
// Every model name explain ships with lives here, so there is exactly one
// place to update when a provider retires one. The previous version of this
// tool hardcoded a model constant in its request builder and became unusable
// when that model was shut down.
//
// ProviderCompatible has no entry: a local server's model names are its own,
// so there is nothing sensible to guess.
var DefaultModels = map[string]string{
	ProviderOpenAI:    "gpt-5.6-luna",
	ProviderAnthropic: "claude-opus-5",
}

// DefaultModel is the model used by the default provider.
var DefaultModel = DefaultModels[DefaultProvider]

// Supported output languages.
const (
	LangEN = "en"
	LangPT = "pt"
)

// DefaultLang is used when no language is configured.
const DefaultLang = LangEN

// Config is the fully resolved configuration for a single run.
type Config struct {
	Provider string `toml:"provider"`
	Model    string `toml:"model"`
	BaseURL  string `toml:"base_url"`
	APIKey   string `toml:"api_key"`
	Lang     string `toml:"lang"`
}

// ErrNoAPIKey is returned when no API key was found in any source. Providers
// that need no credential (a local OpenAI-compatible server, for example) may
// ignore it.
var ErrNoAPIKey = errors.New("no API key found: set EXPLAIN_API_KEY, or add api_key to " + relConfigPath)

const relConfigPath = "~/.config/explain/config.toml"

// Loader reads configuration from an injectable filesystem root and
// environment, so tests do not have to touch the real home directory.
type Loader struct {
	// HomeDir stands in for the user's home directory.
	HomeDir string
	// ConfigDir overrides the derived config directory. Optional.
	ConfigDir string
	// Getenv looks up an environment variable. Defaults to os.Getenv.
	Getenv func(string) string
}

// Load resolves configuration using the real home directory and environment.
func Load() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("locating home directory: %w", err)
	}
	return Loader{HomeDir: home, ConfigDir: os.Getenv("XDG_CONFIG_HOME")}.Load()
}

// Load resolves configuration from the loader's sources.
func (l Loader) Load() (Config, error) {
	getenv := l.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}

	// Model is deliberately left empty here. It is filled in by Resolve, after
	// flags have been applied, so that choosing a provider picks up that
	// provider's default model rather than the previous provider's.
	cfg := Config{Provider: DefaultProvider, Lang: DefaultLang}

	if err := l.applyConfigFile(&cfg); err != nil {
		return Config{}, err
	}
	l.applyLegacyKeyFile(&cfg)
	applyEnv(&cfg, getenv)

	return cfg, nil
}

// ConfigPath returns the path Load reads TOML settings from.
func (l Loader) ConfigPath() string {
	dir := l.ConfigDir
	if dir == "" {
		dir = filepath.Join(l.HomeDir, ".config")
	}
	return filepath.Join(dir, "explain", "config.toml")
}

// applyConfigFile overlays ~/.config/explain/config.toml. A missing file is
// not an error; a malformed one is.
func (l Loader) applyConfigFile(cfg *Config) error {
	path := l.ConfigPath()
	data, err := os.ReadFile(path) //nolint:gosec // path is derived from the user's own home directory
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	var fileCfg Config
	if _, err := toml.Decode(string(data), &fileCfg); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}

	overlay(cfg, fileCfg)
	return nil
}

// applyLegacyKeyFile reads ~/.explainrc, the plain-text key file used by
// explain 1.x. It holds an API key and nothing else. A missing or unreadable
// file is ignored: unlike 1.x, which exited before it could fall back to the
// environment, an absent file here just means "try the next source".
func (l Loader) applyLegacyKeyFile(cfg *Config) {
	data, err := os.ReadFile(filepath.Join(l.HomeDir, ".explainrc")) //nolint:gosec // path is derived from the user's own home directory
	if err != nil {
		return
	}
	if key := strings.TrimSpace(string(data)); key != "" {
		cfg.APIKey = key
	}
}

// applyEnv overlays environment variables. API_KEY is the variable explain 1.x
// documented; it is still honoured so existing Docker invocations keep working.
func applyEnv(cfg *Config, getenv func(string) string) {
	overlay(cfg, Config{
		Provider: getenv("EXPLAIN_PROVIDER"),
		Model:    getenv("EXPLAIN_MODEL"),
		BaseURL:  getenv("EXPLAIN_BASE_URL"),
		Lang:     getenv("EXPLAIN_LANG"),
		APIKey: firstNonEmpty(
			getenv("EXPLAIN_API_KEY"),
			getenv("OPENAI_API_KEY"),
			getenv("API_KEY"),
		),
	})
}

// overlay copies every non-empty field of src onto dst.
func overlay(dst *Config, src Config) {
	for _, f := range []struct {
		dst *string
		src string
	}{
		{&dst.Provider, src.Provider},
		{&dst.Model, src.Model},
		{&dst.BaseURL, src.BaseURL},
		{&dst.APIKey, src.APIKey},
		{&dst.Lang, src.Lang},
	} {
		if src := strings.TrimSpace(f.src); src != "" {
			*f.dst = src
		}
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// Resolve validates the configuration and fills in anything still unset. It
// runs after command-line flags have been applied, because the provider can
// change between Load and here.
func (c *Config) Resolve() error {
	if _, ok := DefaultModels[c.Provider]; !ok && c.Provider != ProviderCompatible {
		return fmt.Errorf("unknown provider %q (supported: %s, %s, %s)",
			c.Provider, ProviderOpenAI, ProviderAnthropic, ProviderCompatible)
	}

	// A base URL means the endpoint is OpenAI-compatible rather than OpenAI
	// itself, so honour it without making the user set both.
	if c.BaseURL != "" && c.Provider == ProviderOpenAI {
		c.Provider = ProviderCompatible
	}

	if c.Provider == ProviderCompatible && c.BaseURL == "" {
		return fmt.Errorf("provider %q needs a base URL: pass --base-url or set base_url in %s",
			ProviderCompatible, relConfigPath)
	}

	if c.Model == "" {
		c.Model = DefaultModels[c.Provider]
	}
	if c.Model == "" {
		return fmt.Errorf("provider %q has no default model: pass --model", c.Provider)
	}

	switch c.Lang {
	case "":
		c.Lang = DefaultLang
	case LangEN, LangPT:
	default:
		return fmt.Errorf("unknown language %q (supported: %s, %s)", c.Lang, LangEN, LangPT)
	}

	return nil
}
