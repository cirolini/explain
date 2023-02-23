package GPTClient

import (
	"fmt"
	"os"
)

var env = "API_KEY"

// RetrieveAIAPIKeyFromEnv retrieves the API key from the environment variable
func RetrieveAIAPIKeyFromEnv() (string, error) {
	key := os.Getenv(env)
	if len(key) < 2 {
		return "", fmt.Errorf("invalid environment variable %s", env)
	}

	prefix := key[:2]
	if prefix != "sk" {
		return "", fmt.Errorf("invalid environment variable %s", env)
	}

	return key, nil

}
