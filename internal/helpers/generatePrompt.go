package helpers

import "fmt"

func GeneratePromptTemplate(args []string) string {
	prompt := fmt.Sprintf("For each item, verify if it is a existant linux terminal command. Then teach how to use it:\n%s\n", generatePrompt(args))
	return prompt
}

func generatePrompt(args []string) string {
	prompt := ""
	for i, arg := range args {
		prompt += fmt.Sprintf("%d. %s\n", i+1, arg)
	}
	return prompt
}
