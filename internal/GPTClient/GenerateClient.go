// Package GPTClient generates a GPT-3 client and a context for the client to use.
package GPTClient

import (
	"context"
	"fmt"

	gogpt "github.com/sashabaranov/go-gpt3"
)

// GenerateClient generates a GPT-3 client
func GenerateClient(key string) (*gogpt.Client, context.Context) {
	fmt.Println(key)
	GPTClient := gogpt.NewClient(key)
	GPTContext := context.Background()

	return GPTClient, GPTContext
}
