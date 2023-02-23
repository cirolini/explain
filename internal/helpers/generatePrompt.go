package helpers

import "fmt"

// GeneratePromptTemplate generates a prompt template for the user to fill in
func GeneratePromptTemplate(args []string) string {
	prompt := fmt.Sprintf("For each item in the list below, if it is a valid Linux terminal command, explain it in details, and show an example. \n%s\n", generatePrompt(args))
	return prompt
}

func generatePrompt(args []string) string {
	prompt := ""
	for i, arg := range args {
		prompt += fmt.Sprintf("%d. %s\n", i+1, arg)
	}
	return prompt
}
