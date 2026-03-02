package main

import (
	"fmt"
	"log"

	"github.com/cdg-me/browsii/internal/client"
	"github.com/spf13/cobra"
)

func init() {
	jsCmd := &cobra.Command{
		Use:   "js <script>",
		Short: "Executes JavaScript in the active browser tab and returns the JSON result",
		Long: `Executes JavaScript in the active browser tab and returns the JSON-serialized result.

Accepts either a bare expression or a function expression:
  browsii js "document.title"
  browsii js "2 + 2"
  browsii js "({href: location.href, title: document.title})"
  browsii js "() => document.querySelectorAll('a').length"
  browsii js "function() { return window.scrollY; }"

Bare expressions are automatically wrapped in an arrow function before evaluation.`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			script := args[0]
			payload := map[string]string{"script": script}

			resp, err := client.SendCommand(port, "js", payload)
			if err != nil {
				log.Fatalf("JS Evaluation failed: %v", err)
			}

			// The response is already serialized JSON from the browser evaluation
			fmt.Println(string(resp))
		},
	}

	rootCmd.AddCommand(jsCmd)
}
