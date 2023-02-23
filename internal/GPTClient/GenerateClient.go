// Package GPTClient generates a GPT-3 client and a context for the client to use.
package GPTClient

import (
	"context"

	gogpt "github.com/sashabaranov/go-gpt3"
)

// GenerateClient generates a GPT-3 client
func GenerateClient(key string) (*gogpt.Client, context.Context) {
	GPTClient := gogpt.NewClient(key)
	GPTContext := context.Background()

	return GPTClient, GPTContext
}
