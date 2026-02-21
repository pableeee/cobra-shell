// cobra-shell wraps any Cobra CLI binary in an interactive shell.
//
// Usage:
//
//	cobra-shell --binary <path> [--prompt <string>] [--history <file>] [--timeout <duration>] [--env-builtin <name>]
//
// Examples:
//
//	cobra-shell --binary kubectl --prompt "k8s> "
//	cobra-shell --binary gh
//	cobra-shell --binary ./myapp --timeout 2s
//	cobra-shell --binary ./myapp --env-builtin env
package main

import (
	"fmt"
	"os"
	"time"

	cobrashell "github.com/pable/cobra-shell"
	"github.com/spf13/cobra"
)

func main() {
	var (
		binary     string
		prompt     string
		history    string
		timeout    time.Duration
		envBuiltin string
	)

	root := &cobra.Command{
		Use:   "cobra-shell",
		Short: "Start an interactive shell for any Cobra CLI",
		Long: `cobra-shell wraps any Cobra binary in an interactive shell with tab
completion (via __completeNoDesc) and persistent command history.`,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cobrashell.New(cobrashell.Config{
				BinaryPath:        binary,
				Prompt:            prompt,
				HistoryFile:       history,
				CompletionTimeout: timeout,
				EnvBuiltin:        envBuiltin,
			}).Run()
		},
	}

	root.Flags().StringVarP(&binary, "binary", "b", "", "Path or name of the Cobra binary to wrap (required)")
	root.Flags().StringVarP(&prompt, "prompt", "p", "", `Prompt string (default: "> ")`)
	root.Flags().StringVar(&history, "history", "", "History file path (default: ~/.<binary>_history)")
	root.Flags().DurationVar(&timeout, "timeout", 500*time.Millisecond, "Tab completion timeout")
	root.Flags().StringVar(&envBuiltin, "env-builtin", "", `Enable a built-in env command with this name (e.g. "env"). Supports: list, set KEY VALUE, unset KEY`)
	_ = root.MarkFlagRequired("binary")

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
