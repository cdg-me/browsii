package main

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/client"
)

func init() {
	reloadCmd := &cobra.Command{
		Use:   "reload",
		Short: "Reloads the active page",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			_, err := client.SendCommand(port, "reload", nil)
			if err != nil {
				log.Fatalf("Reload failed: %v", err)
			}

			fmt.Println("Successfully reloaded page")
		},
	}

	rootCmd.AddCommand(reloadCmd)
}
