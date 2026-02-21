// testbin is a minimal Cobra binary used by cobra-shell integration tests.
// It exposes known commands and completions so tests can make deterministic
// assertions about __completeNoDesc output and execution behaviour.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "testbin",
		Short: "cobra-shell integration test binary",
	}

	var name string
	greet := &cobra.Command{
		Use:   "greet",
		Short: "Print a greeting",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Hello, %s!\n", name)
			return nil
		},
	}
	greet.Flags().StringVar(&name, "name", "world", "Name to greet")
	root.AddCommand(greet)

	root.AddCommand(&cobra.Command{
		Use:   "fail",
		Short: "Exit with a non-zero status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("intentional failure")
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "echo",
		Short: "Print arguments",
		Run: func(cmd *cobra.Command, args []string) {
			for _, a := range args {
				fmt.Println(a)
			}
		},
	})

	root.AddCommand(&cobra.Command{
		Use:    "hidden",
		Short:  "Hidden command (should not appear in completions)",
		Hidden: true,
	})

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
