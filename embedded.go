package cobrashell

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/chzyer/readline"
	"github.com/google/shlex"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// CompletionFunc provides dynamic completion candidates for an embedded-mode
// command. It is called in addition to the command's own ValidArgsFunction and
// static subcommand/flag enumeration.
//
// args contains the tokens already accepted by the matched command; toComplete
// is the partial word being completed. The function should return all
// candidates — filtering by toComplete prefix is the function's responsibility.
//
// The signature mirrors cobra's own ValidArgsFunction for consistency.
type CompletionFunc func(args []string, toComplete string) []string

// EmbeddedConfig configures a [NewEmbedded] shell.
//
// All fields are optional except RootCmd. Zero values are replaced with
// sensible defaults by [NewEmbedded].
type EmbeddedConfig struct {
	// RootCmd is the Cobra command tree to wrap in-process. Must not be nil.
	RootCmd *cobra.Command

	// Prompt, HistoryFile, and CompletionTimeout behave identically to the
	// corresponding fields in [Config].
	Prompt            string
	HistoryFile       string
	CompletionTimeout time.Duration

	// Hooks contains optional lifecycle callbacks.
	Hooks EmbeddedHooks

	// DynamicCompletions maps a command name to a [CompletionFunc] that
	// returns live candidates sourced from in-process state (e.g. database
	// records, cache keys). It is called in addition to the command's own
	// ValidArgsFunction and static subcommand/flag enumeration.
	DynamicCompletions map[string]CompletionFunc
}

// EmbeddedHooks contains optional lifecycle callbacks for an [EmbeddedShell].
// All fields are optional; nil functions are silently skipped.
//
// EmbeddedHooks is a distinct type from [Hooks] because OnStart receives an
// *EmbeddedShell rather than a *Shell — the two shell types are not
// interchangeable.
type EmbeddedHooks struct {
	// BeforeExec is called before each command is executed. Return a non-nil
	// error to cancel execution; the message is printed to stderr and the
	// shell continues. Return nil to allow execution to proceed.
	BeforeExec func(args []string) error

	// AfterExec is called after each command completes with its exit code.
	// exitCode is 0 on success and 1 when cobra.Command.Execute returns an
	// error.
	AfterExec func(args []string, exitCode int)

	// OnStart is called once when the shell starts, before the first prompt.
	// Useful for printing a welcome banner.
	OnStart func(shell *EmbeddedShell)

	// OnExit is called once on a clean exit (Ctrl-D or the "exit" command).
	OnExit func()
}

// EmbeddedShell runs a Cobra command tree interactively within the same
// process. Create one with [NewEmbedded] and start it with [Run].
//
// Unlike [Shell] (subprocess mode), EmbeddedShell invokes
// cobra.Command.Execute directly, which allows commands to share in-process
// state such as database connections or caches. Tab completion is driven by
// walking the command tree and calling ValidArgsFunction rather than by
// spawning a __completeNoDesc subprocess.
type EmbeddedShell struct {
	cfg     EmbeddedConfig
	initErr error
}

// NewEmbedded creates an EmbeddedShell from cfg. If cfg.RootCmd is nil the
// error is stored and returned by [EmbeddedShell.Run]. All zero-value fields
// are replaced with defaults before Run is called.
//
// NewEmbedded never returns nil.
func NewEmbedded(cfg EmbeddedConfig) *EmbeddedShell {
	s := &EmbeddedShell{}
	if cfg.RootCmd == nil {
		s.initErr = errors.New("cobra-shell: EmbeddedConfig.RootCmd must not be nil")
		s.cfg = cfg
		return s
	}

	if cfg.Prompt == "" {
		cfg.Prompt = defaultPrompt
	}
	if cfg.CompletionTimeout == 0 {
		cfg.CompletionTimeout = defaultCompletionTimeout
	}
	if cfg.HistoryFile == "" {
		// RootCmd.Name() returns the bare command name (no path), which is
		// exactly what defaultHistoryFilePath expects.
		cfg.HistoryFile = defaultHistoryFilePath(cfg.RootCmd.Name())
	}

	s.cfg = cfg
	return s
}

// Run starts the interactive shell loop. It blocks until the user exits via
// Ctrl-D or the built-in "exit" command.
//
// Run returns a non-nil error if:
//   - RootCmd was nil (error stored by [NewEmbedded])
//   - readline fails to initialise
//
// A clean exit returns nil.
func (s *EmbeddedShell) Run() error {
	if s.initErr != nil {
		return s.initErr
	}

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          s.cfg.Prompt,
		HistoryFile:     s.cfg.HistoryFile,
		AutoComplete:    &embeddedCompleter{shell: s},
		InterruptPrompt: "",
		EOFPrompt:       "exit",
	})
	if err != nil {
		return fmt.Errorf("cobra-shell: initialise readline: %w", err)
	}
	defer rl.Close()

	if s.cfg.Hooks.OnStart != nil {
		s.cfg.Hooks.OnStart(s)
	}

	for {
		line, err := rl.Readline()
		if err == io.EOF {
			break
		}
		if errors.Is(err, readline.ErrInterrupt) {
			continue
		}
		if err != nil {
			return fmt.Errorf("cobra-shell: readline: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "exit" {
			break
		}

		s.execute(line)
	}

	if s.cfg.Hooks.OnExit != nil {
		s.cfg.Hooks.OnExit()
	}
	return nil
}

// execute tokenises line, runs BeforeExec, resets the command tree flags,
// calls cobra.Command.Execute, and runs AfterExec.
func (s *EmbeddedShell) execute(line string) {
	tokens, err := shlex.Split(line)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cobra-shell: parse error: %v\n", err)
		return
	}
	if len(tokens) == 0 {
		return
	}

	if s.cfg.Hooks.BeforeExec != nil {
		if err := s.cfg.Hooks.BeforeExec(tokens); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
	}

	// Reset all flag values to their defaults before each execution so that
	// flags set by a previous command do not bleed into the current one.
	resetCommandTree(s.cfg.RootCmd)

	s.cfg.RootCmd.SetArgs(tokens)
	s.cfg.RootCmd.SetOut(os.Stdout)
	s.cfg.RootCmd.SetErr(os.Stderr)
	s.cfg.RootCmd.SetIn(os.Stdin)

	exitCode := 0
	if err := s.cfg.RootCmd.Execute(); err != nil {
		exitCode = 1
	}

	if s.cfg.Hooks.AfterExec != nil {
		s.cfg.Hooks.AfterExec(tokens, exitCode)
	}
}

// resetCommandTree resets every flag in cmd and its descendants to its default
// value and clears the Changed marker. This must be called before each
// Execute() to prevent flag state from one shell command bleeding into the
// next.
func resetCommandTree(cmd *cobra.Command) {
	reset := func(f *pflag.Flag) {
		if f.Changed {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		}
	}
	cmd.Flags().VisitAll(reset)
	cmd.PersistentFlags().VisitAll(reset)
	for _, child := range cmd.Commands() {
		resetCommandTree(child)
	}
}
