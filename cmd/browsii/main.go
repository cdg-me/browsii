package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags "-X main.Version=x.y.z".
var Version = "dev"

var port int

var rootCmd = &cobra.Command{
	Use:     "browsii",
	Version: Version,
	Short:   "A headless browser automation CLI designed for LLMs",
	Long: `browsii is a fast, single-binary CLI wrapper around go-rod.
It is designed to give LLMs and automated scripts robust, stateful control over
browser instances for tasks like scraping, UI verification, and research.`,
}

func init() {
	rootCmd.PersistentFlags().IntVarP(&port, "port", "p", 8000, "Port of the running daemon")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func main() {
	Execute()
}
