package main

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/client"
)

func init() {
	uploadCmd := &cobra.Command{
		Use:   "upload <selector> <filepath>",
		Short: "Sets a file on an input[type=file] element",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			selector := args[0]
			filePath := args[1]
			payload := map[string]interface{}{
				"selector": selector,
				"files":    []string{filePath},
			}

			_, err := client.SendCommand(port, "upload", payload)
			if err != nil {
				log.Fatalf("Upload failed: %v", err)
			}

			fmt.Printf("Successfully set file %s on %s\n", filePath, selector)
		},
	}

	rootCmd.AddCommand(uploadCmd)
}
