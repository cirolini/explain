package main

import (
	"cirolini/explain/internal/GPTClient"
	"cirolini/explain/internal/cobraClient"
	"context"
	"fmt"
	"os"

	gogpt "github.com/sashabaranov/go-gpt3"
)

var GoGPTClient *gogpt.Client
var GoGPTContext context.Context

func init() {
	key, err := GPTClient.RetrieveAIAPIKey()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	GoGPTClient, GoGPTContext = GPTClient.GenerateClient(key)
}

func main() {
	command := cobraClient.GenerateCommand(cobraClient.GenerateCommandArgs{
		GPTClient:  GoGPTClient,
		GPTContext: GoGPTContext,
	})

	err := command.Execute()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
