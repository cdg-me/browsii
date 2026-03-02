package main

import (
	"fmt"
	"log"

	"github.com/cdg-me/browsii/internal/client"
	"github.com/spf13/cobra"
)

var contextName string

func init() {
	contextCmd := &cobra.Command{
		Use:   "context",
		Short: "Browser context management (create, switch)",
	}

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Creates a new isolated browser context (incognito)",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			payload := map[string]string{"name": contextName}
			_, err := client.SendCommand(port, "context/create", payload)
			if err != nil {
				log.Fatalf("Context create failed: %v", err)
			}
			fmt.Printf("Created and switched to context %q\n", contextName)
		},
	}
	createCmd.Flags().StringVar(&contextName, "name", "", "Name for the new context")
	createCmd.MarkFlagRequired("name")

	switchCmd := &cobra.Command{
		Use:   "switch <name>",
		Short: "Switches to a named context (use 'default' for main context)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			payload := map[string]string{"name": args[0]}
			_, err := client.SendCommand(port, "context/switch", payload)
			if err != nil {
				log.Fatalf("Context switch failed: %v", err)
			}
			fmt.Printf("Switched to context %q\n", args[0])
		},
	}

	contextCmd.AddCommand(createCmd)
	contextCmd.AddCommand(switchCmd)

	rootCmd.AddCommand(contextCmd)
}
