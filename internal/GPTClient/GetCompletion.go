package GPTClient

import (
	"context"
	"fmt"
	"os"

	gogpt "github.com/sashabaranov/go-gpt3"
)

type GetCompletionArgs struct {
	GPTClient  *gogpt.Client
	GPTContext context.Context
	Request    gogpt.CompletionRequest
}

func GetCompletion(args GetCompletionArgs) string {
	completion, err := args.GPTClient.CreateCompletion(args.GPTContext, args.Request)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	return completion.Choices[0].Text
}
