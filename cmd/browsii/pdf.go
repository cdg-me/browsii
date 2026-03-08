package main

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/client"
)

func init() {
	pdfCmd := &cobra.Command{
		Use:   "pdf <filename.pdf>",
		Short: "Renders the active page as a PDF",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			filename := args[0]
			payload := map[string]string{"filename": filename}

			_, err := client.SendCommand(port, "pdf", payload)
			if err != nil {
				log.Fatalf("PDF generation failed: %v", err)
			}

			fmt.Printf("Successfully saved PDF to %s\n", filename)
		},
	}

	rootCmd.AddCommand(pdfCmd)
}
