package GPTClient

import (
	"os"
)

var AI_API_KEY_LENGTH = 52

func RetriveAIAPIKey() string {
	key := os.Getenv("API_KEY")
	return key
}
