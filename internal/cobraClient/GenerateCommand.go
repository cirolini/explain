package cobraClient

import (
	"cirolini/explain/internal/GPTClient"
	"cirolini/explain/internal/helpers"
	"context"
	"fmt"

	gogpt "github.com/sashabaranov/go-gpt3"
	"github.com/spf13/cobra"
)

type GenerateCommandArgs struct {
	GPTClient  *gogpt.Client
	GPTContext context.Context
}

func GenerateCommand(generateCommandArgs GenerateCommandArgs) cobra.Command {
	cmd := cobra.Command{
		Use:   "explain [prompt]",
		Short: "Explain a command",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			prompt := helpers.GeneratePromptTemplate(args)
			req := GPTClient.GenerateRequest(prompt)
			completion := GPTClient.GetCompletion(GPTClient.GetCompletionArgs{
				GPTClient:  generateCommandArgs.GPTClient,
				GPTContext: generateCommandArgs.GPTContext,
				Request:    req,
			})
			fmt.Printf("\n---\n%s\n\n---\n", completion)
		},
	}

	return cmd
}
