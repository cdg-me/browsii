package main

import (
	"fmt"

	"github.com/spf13/cobra"

	browsii "github.com/cdg-me/browsii"
)

func init() {
	quickstartCmd := &cobra.Command{
		Use:   "quickstart",
		Short: "Print the browsii quickstart guide",
		Long:  `Prints the full browsii quickstart guide — modes, CLI reference, WASM SDK, and Go client API.`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Print(browsii.Quickstart)
		},
	}

	rootCmd.AddCommand(quickstartCmd)
}
