package main

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/client"
)

var (
	screenshotElement  string
	screenshotFullPage bool
)

func init() {
	screenshotCmd := &cobra.Command{
		Use:   "screenshot <filename.png>",
		Short: "Captures a screenshot of the active page",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			filename := args[0]
			payload := map[string]interface{}{
				"filename": filename,
				"element":  screenshotElement,
				"fullPage": screenshotFullPage,
			}

			_, err := client.SendCommand(port, "screenshot", payload)
			if err != nil {
				log.Fatalf("Screenshot failed: %v", err)
			}

			fmt.Printf("Successfully saved screenshot to %s\n", filename)
		},
	}

	screenshotCmd.Flags().StringVar(&screenshotElement, "element", "", "CSS selector to screenshot a specific element")
	screenshotCmd.Flags().BoolVar(&screenshotFullPage, "full-page", false, "Capture the full scrollable page")

	rootCmd.AddCommand(screenshotCmd)
}
