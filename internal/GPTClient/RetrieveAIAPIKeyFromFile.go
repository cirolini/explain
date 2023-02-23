package GPTClient

import (
	"fmt"
	"os"
	"path"
	"strings"
)

var file = ".explainrc"

func RetrieveAIAPIKeyFromFile() (string, error) {
	dir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	bytes, err := os.ReadFile(path.Join(dir, file))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	key := strings.TrimSuffix(string(bytes), "\n")

	return key, nil

}
