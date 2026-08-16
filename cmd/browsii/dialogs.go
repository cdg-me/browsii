package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/client"
)

var (
	dialogPolicy     string
	dialogPromptText string
	dialogClear      bool
)

func init() {
	dialogsCmd := &cobra.Command{
		Use:   "dialogs",
		Short: "Shows auto-handled JavaScript dialogs and sets the handling policy",
		Long: `JavaScript dialogs (alert, confirm, prompt, beforeunload) are auto-handled
so they never stall the session. This command lists dialogs handled recently
and optionally changes the policy.

  browsii dialogs                      # show policy + recent dialogs
  browsii dialogs --policy accept      # accept confirm()/prompt() from now on
  browsii dialogs --prompt-text "hi"   # text typed into accepted prompt() dialogs
  browsii dialogs --clear              # forget recent history

Dialogs are also reported inline by click/press/navigate/js when the action
triggers one. Default policy: dismiss (confirm() returns false, beforeunload
cancels navigation).`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			payload := map[string]any{}
			if dialogPolicy != "" {
				payload["policy"] = dialogPolicy
			}
			if dialogPromptText != "" {
				payload["prompt_text"] = dialogPromptText
			}
			if dialogClear {
				payload["clear"] = true
			}

			resp, err := client.SendCommand(port, "dialogs", payload)
			if err != nil {
				log.Fatalf("Dialogs failed: %v", err)
			}

			var state struct {
				Policy     string `json:"policy"`
				PromptText string `json:"prompt_text"`
				Recent     []struct {
					Type     string  `json:"type"`
					Message  string  `json:"message"`
					Accepted bool    `json:"accepted"`
					Tab      int     `json:"tab"`
					TS       float64 `json:"ts"`
				} `json:"recent"`
			}
			if err := json.Unmarshal(resp, &state); err != nil {
				log.Fatalf("Dialogs failed: unexpected response: %v", err)
			}

			fmt.Printf("Dialog policy: %s (prompt text: %q)\n", state.Policy, state.PromptText)
			if len(state.Recent) == 0 {
				fmt.Println("No dialogs recorded.")
				return
			}
			for _, d := range state.Recent {
				disposition := "dismissed"
				if d.Accepted {
					disposition = "accepted"
				}
				fmt.Printf("  %s %s: %q (tab %d)\n", d.Type, disposition, d.Message, d.Tab)
			}
		},
	}

	dialogsCmd.Flags().StringVar(&dialogPolicy, "policy", "", "Handling policy for new dialogs: accept or dismiss")
	dialogsCmd.Flags().StringVar(&dialogPromptText, "prompt-text", "", "Text typed into prompt() dialogs when accepting")
	dialogsCmd.Flags().BoolVar(&dialogClear, "clear", false, "Clear the recent-dialog history")

	rootCmd.AddCommand(dialogsCmd)
}
