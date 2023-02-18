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
	dir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	content, err := os.ReadFile(fmt.Sprintf("%s/apikey.txt", dir))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	GoGPTClient, GoGPTContext = GPTClient.GenerateClient(string(content))
}

func main() {
	command := cobraClient.GenerateCommand(cobraClient.GenerateCommandArgs{
		GPTClient:  GoGPTClient,
		GPTContext: GoGPTContext,
	})

	command.Execute()
}
