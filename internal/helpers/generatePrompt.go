package helpers

import "fmt"

func GeneratePromptTemplate(args []string) string {
	prompt := fmt.Sprintf("For each item, explain in details what they do in a Linux terminal\n%s\n", generatePrompt(args))
	return prompt
}

func generatePrompt(args []string) string {
	prompt := ""
	for i, arg := range args {
		prompt += fmt.Sprintf("%d. %s\n", i+1, arg)
	}
	return prompt
}
