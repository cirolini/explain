package GPTClient

import (
	"context"

	gogpt "github.com/sashabaranov/go-gpt3"
)

func GenerateClient(AIAPIKey string) (*gogpt.Client, context.Context) {
	GPTClient := gogpt.NewClient(AIAPIKey)
	GPTContext := context.Background()

	return GPTClient, GPTContext
}
