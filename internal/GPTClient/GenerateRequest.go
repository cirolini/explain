package GPTClient

import (
	gogpt "github.com/sashabaranov/go-gpt3"
)

// GenerateRequest generates a request for GPT-3
func GenerateRequest(prompt string) gogpt.CompletionRequest {
	request := gogpt.CompletionRequest{
		Model:       gogpt.GPT3TextDavinci003,
		MaxTokens:   4000,
		Temperature: 0.5,
		Prompt:      prompt,
	}

	return request
}
