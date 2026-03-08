package main

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/client"
)

func init() {
	sessionCmd := &cobra.Command{
		Use:   "session",
		Short: "Session management (new, save, resume, list, delete)",
	}

	newCmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Closes all tabs and starts a fresh session",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			payload := map[string]string{"name": args[0]}
			_, err := client.SendCommand(port, "session/new", payload)
			if err != nil {
				log.Fatalf("Session new failed: %v", err)
			}
			fmt.Printf("Started new session %q\n", args[0])
		},
	}

	saveCmd := &cobra.Command{
		Use:   "save <name>",
		Short: "Saves the current session (tabs, scroll positions) to disk",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			payload := map[string]string{"name": args[0]}
			_, err := client.SendCommand(port, "session/save", payload)
			if err != nil {
				log.Fatalf("Session save failed: %v", err)
			}
			fmt.Printf("Session %q saved\n", args[0])
		},
	}

	resumeCmd := &cobra.Command{
		Use:   "resume <name>",
		Short: "Restores a saved session (reopens tabs, restores scroll positions)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			payload := map[string]string{"name": args[0]}
			_, err := client.SendCommand(port, "session/resume", payload)
			if err != nil {
				log.Fatalf("Session resume failed: %v", err)
			}
			fmt.Printf("Session %q resumed\n", args[0])
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "Lists all saved sessions",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			resp, err := client.SendCommand(port, "session/list", nil)
			if err != nil {
				log.Fatalf("Session list failed: %v", err)
			}
			fmt.Print(string(resp))
		},
	}

	deleteCmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Deletes a saved session",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			payload := map[string]string{"name": args[0]}
			_, err := client.SendCommand(port, "session/delete", payload)
			if err != nil {
				log.Fatalf("Session delete failed: %v", err)
			}
			fmt.Printf("Session %q deleted\n", args[0])
		},
	}

	sessionCmd.AddCommand(newCmd)
	sessionCmd.AddCommand(saveCmd)
	sessionCmd.AddCommand(resumeCmd)
	sessionCmd.AddCommand(listCmd)
	sessionCmd.AddCommand(deleteCmd)

	rootCmd.AddCommand(sessionCmd)
}
