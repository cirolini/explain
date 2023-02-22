package GPTClient

import (
	"os"
)

func RetriveAIAPIKey() string {
	key := os.Getenv("API_KEY")
	return key
}
