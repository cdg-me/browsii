package main

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/client"
)

func init() {
	backCmd := &cobra.Command{
		Use:   "back",
		Short: "Navigates back in browser history",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			_, err := client.SendCommand(port, "back", nil)
			if err != nil {
				log.Fatalf("Back navigation failed: %v", err)
			}

			fmt.Println("Successfully navigated back")
		},
	}
	rootCmd.AddCommand(backCmd)

	forwardCmd := &cobra.Command{
		Use:   "forward",
		Short: "Navigates forward in browser history",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			_, err := client.SendCommand(port, "forward", nil)
			if err != nil {
				log.Fatalf("Forward navigation failed: %v", err)
			}

			fmt.Println("Successfully navigated forward")
		},
	}
	rootCmd.AddCommand(forwardCmd)
}
