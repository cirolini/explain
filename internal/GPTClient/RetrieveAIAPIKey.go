package GPTClient

import (
	"fmt"
	"os"
)

func RetriveAIAPIKey() (string, error) {
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
