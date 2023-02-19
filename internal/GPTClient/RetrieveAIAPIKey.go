package GPTClient

import (
	"fmt"
	"os"
)

func RetriveAIAPIKey() (string, error) {
	keyFromEnv := os.Getenv("API_KEY")
	if keyFromEnv != "" {
		return keyFromEnv, nil
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	key, err := os.ReadFile(fmt.Sprintf("%s/.explainrc", dir))
	if err != nil {
		return "", err
	}
	return string(key), nil
}
