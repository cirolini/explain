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

// newProvider selects an adapter. Phase 0 ships OpenAI, which also reaches any
// OpenAI-compatible server through base_url.
func newProvider(cfg config.Config) (llm.Provider, error) {
	switch cfg.Provider {
	case "openai", "openai-compatible":
		p, err := llm.NewOpenAI(llm.Options{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,
		})
		if errors.Is(err, llm.ErrNoAPIKey) {
			return nil, config.ErrNoAPIKey
		}
		return p, err
	default:
		return nil, fmt.Errorf("unknown provider %q (supported: openai)", cfg.Provider)
	}
}
