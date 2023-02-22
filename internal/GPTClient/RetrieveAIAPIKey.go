package GPTClient

import (
	"fmt"
	"os"
)

var fileName = ".explainrc"

func RetrieveAIAPIKey() (key string, retrieveAIAPIKeyError error) {
	fmt.Println("Retrieving API key from environment variable...")
	key = os.Getenv("API_KEY")
	if key == "" {
		fmt.Println("Environment variable API_KEY not found.")
	}
	keyPrefix := key[0:2]
	if keyPrefix == "sk" {
		return key, nil
	}
	fmt.Println("Could not retrieve API key from environment variable.")

	fmt.Println("Retrieving API key from file...")
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	filePath := fmt.Sprintf("%s/%s", homeDir, fileName)
	file, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	key = string(file)

	return key, nil
}
