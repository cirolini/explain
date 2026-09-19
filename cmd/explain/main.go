// Command explain describes a shell command before you run it.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/cirolini/explain/internal/cli"
	"github.com/cirolini/explain/internal/config"
	"github.com/cirolini/explain/internal/llm"
)

// version is overwritten at build time via -ldflags.
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "explain: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	cmd := cli.NewCommand(cli.Options{
		Config:      cfg,
		NewProvider: newProvider,
		Version:     version,
	})

	return cmd.ExecuteContext(ctx)
}

// newProvider selects an adapter for the resolved configuration. Config.Resolve
// has already rejected unknown providers, so the default case is unreachable in
// practice -- it exists so adding a provider constant without an adapter fails
// loudly rather than silently falling through to OpenAI.
func newProvider(cfg config.Config) (llm.Provider, error) {
	opts := llm.Options{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, Model: cfg.Model}

	var (
		p   llm.Provider
		err error
	)
	switch cfg.Provider {
	case config.ProviderOpenAI, config.ProviderCompatible:
		p, err = llm.NewOpenAI(opts)
	case config.ProviderAnthropic:
		p, err = llm.NewAnthropic(opts)
	default:
		return nil, fmt.Errorf("no adapter for provider %q", cfg.Provider)
	}

	if errors.Is(err, llm.ErrNoAPIKey) {
		return nil, config.ErrNoAPIKey
	}
	return p, err
}
