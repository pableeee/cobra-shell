package cobrashell

import "github.com/spf13/cobra"

// Command returns a *cobra.Command with Use "shell" that starts an interactive
// shell wrapping the binary described by cfg. It is intended to be added as a
// subcommand of a Cobra CLI so that users can run `mybinary shell` to enter an
// interactive session:
//
//	rootCmd.AddCommand(cobrashell.Command(cobrashell.Config{
//	    BinaryPath: os.Args[0],
//	    Prompt:     "myapp> ",
//	}))
//
// When BinaryPath is set to os.Args[0], the shell wraps the running binary
// itself. New resolves the path to an absolute path immediately, so it remains
// valid even if the process changes its working directory.
func Command(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "shell",
		Short: "Start an interactive shell",
		Long: "Start an interactive shell that wraps this binary's commands " +
			"with tab completion and persistent history.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return New(cfg).Run()
		},
	}
}
