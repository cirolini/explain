package GPTClient

import (
	"context"

	gogpt "github.com/sashabaranov/go-gpt3"
)

func GenerateClient(key string) (*gogpt.Client, context.Context) {
	GPTClient := gogpt.NewClient(key)
	GPTContext := context.Background()

	return GPTClient, GPTContext
}
