package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

var cmd = &cobra.Command{
	Use:   "explain [command]",
	Short: "Explain a command",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Concatenate command line arguments into a single string
		command := strings.Join(args, " ")

		// Build JSON payload
		payload := map[string]interface{}{
			"prompt":      fmt.Sprintf("Explain the command %s", command),
			"model":       "text-davinci-002",
			"temperature": 0.5,
			"max_tokens":  100,
		}
		jsonValue, _ := json.Marshal(payload)

		// Make API call to OpenAI
		httpClient := &http.Client{}
		req, _ := http.NewRequest("POST", "https://api.openai.com/v1/completions", bytes.NewBuffer(jsonValue))
		req.Header.Add("Content-Type", "application/json")
		req.Header.Add("Authorization", "Bearer YOUR_API_KEY")
		resp, _ := httpClient.Do(req)
		defer resp.Body.Close()

		// Unmarshal JSON response
		var response map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&response)
		fmt.Println(response["choices"][0]["text"])
	},
}

func main() {
	cmd.Execute()
}
