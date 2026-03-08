package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/wasm"
)

var runCmd = &cobra.Command{
	Use:   "run <script.go | script.wasm>",
	Short: "Executes an automation script inside the WASM sandbox",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		input := args[0]
		bBytes, err := compileOrLoad(input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "{\"error\": \"failed to load script: %v\", \"type\": \"host_runtime_error\"}\n", err)
			os.Exit(2)
		}

		runtime := wasm.NewRuntime(port)
		exitCode := runtime.Run(context.Background(), bBytes)
		os.Exit(exitCode)
	},
}

func compileOrLoad(input string) ([]byte, error) {
	if strings.HasSuffix(input, ".wasm") {
		return os.ReadFile(input)
	}
	if !strings.HasSuffix(input, ".go") {
		return nil, fmt.Errorf("unsupported file extension, must be .go or .wasm")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	// Create a unique temporary compilation directory to avoid collisions
	buildDir := filepath.Join(home, ".browsii", "tmp", fmt.Sprintf("build-%d", os.Getpid()))
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return nil, err
	}
	defer os.RemoveAll(buildDir) //nolint:errcheck

	scriptBytes, err := os.ReadFile(input)
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(filepath.Join(buildDir, "main.go"), scriptBytes, 0644); err != nil {
		return nil, err
	}

	// Rely on the reference SDK mapped via install-runtimes
	sdkDir := filepath.Join(home, ".browsii", "sdk", "go")
	modContent := fmt.Sprintf("module script\n\ngo 1.25\n\nrequire browsii/sdk v0.0.0\nreplace browsii/sdk => %s\n", sdkDir)
	if err := os.WriteFile(filepath.Join(buildDir, "go.mod"), []byte(modContent), 0644); err != nil {
		return nil, err
	}

	outWasm := filepath.Join(buildDir, "out.wasm")
	c := exec.Command("tinygo", "build", "-o", outWasm, "-target", "wasip1", ".") //nolint:noctx
	c.Dir = buildDir
	c.Stdout = os.Stderr
	c.Stderr = os.Stderr

	if err := c.Run(); err != nil {
		return nil, fmt.Errorf("tinygo build failed: %w", err)
	}

	return os.ReadFile(outWasm)
}

func init() {
	rootCmd.AddCommand(runCmd)
}
