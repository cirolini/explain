package cobraClient

import (
	"cirolini/explain/internal/GPTClient"
	"context"
	"fmt"

	gogpt "github.com/sashabaranov/go-gpt3"
	"github.com/spf13/cobra"
)

type GenerateCommandArgs struct {
	GPTClient  *gogpt.Client
	GPTContext context.Context
}

func GeneratePrompt(args []string) string {
	prompt := ""
	for i, arg := range args {
		prompt += fmt.Sprintf("%d. %s\n", i+1, arg)
	}
	return prompt
}

func GenerateCommand(generateCommandArgs GenerateCommandArgs) cobra.Command {
	cmd := cobra.Command{
		Use:   "explain [prompt]",
		Short: "Explain a command",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			prompt := fmt.Sprintf("For each item, verify if it is a existant linux terminal command. Then teach how to use it:\n%s\n", GeneratePrompt(args))
			req := GPTClient.GenerateRequest(prompt)
			completion := GPTClient.GetCompletion(GPTClient.GetCompletionArgs{
				GPTClient:  generateCommandArgs.GPTClient,
				GPTContext: generateCommandArgs.GPTContext,
				Request:    req,
			})
			fmt.Printf("\n------------------------\n%s\n\n----------------------------\n", completion)
		},
	}

	return cmd
}
