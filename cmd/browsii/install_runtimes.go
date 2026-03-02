package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/cdg-me/browsii/sdk"
	"github.com/spf13/cobra"
)

var installRuntimesCmd = &cobra.Command{
	Use:   "install-runtimes",
	Short: "Extracts reference SDKs locally to compile WASM guests",
	Run: func(cmd *cobra.Command, args []string) {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Printf("Failed to get homedir: %v\n", err)
			os.Exit(1)
		}

		targetDir := filepath.Join(home, ".browsii", "sdk")

		// Force clean the directory to ensure modified embed files aren't quietly skipped
		os.RemoveAll(targetDir)

		if err := os.MkdirAll(targetDir, 0755); err != nil {
			fmt.Printf("Failed to create sdks dir: %v\n", err)
			os.Exit(1)
		}

		err = fs.WalkDir(sdk.FS, "go", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			// SDK files are embedded under "go/*"
			outPath := filepath.Join(targetDir, path)

			if d.IsDir() {
				return os.MkdirAll(outPath, 0755)
			}

			b, err := sdk.FS.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(outPath, b, 0644)
		})

		if err != nil {
			fmt.Printf("Failed writing SDKs: %v\n", err)
			os.Exit(1)
		}

		// Also generate an immediate dummy go.mod for the sdk folder itself so the replacer works seamlessly
		sdkModPath := filepath.Join(targetDir, "go", "go.mod")
		if _, err := os.Stat(sdkModPath); os.IsNotExist(err) {
			os.WriteFile(sdkModPath, []byte("module browsii/sdk\n\ngo 1.22\n"), 0644)
		}

		fmt.Printf("Successfully installed Reference SDKs to: %s\n", targetDir)
	},
}

func init() {
	rootCmd.AddCommand(installRuntimesCmd)
}
