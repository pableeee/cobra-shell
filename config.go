// Package cobrashell wraps any Cobra CLI binary in an interactive readline
// shell with tab completion and persistent history. The target binary is
// treated as a black box: completion candidates are discovered at runtime by
// invoking the binary's hidden __completeNoDesc command, which every
// cobra ≥ 1.2 binary exposes automatically.
//
// Minimal usage:
//
//	sh := cobrashell.New(cobrashell.Config{
//	    BinaryPath: "/usr/local/bin/kubectl",
//	    Prompt:     "k8s> ",
//	})
//	if err := sh.Run(); err != nil {
//	    log.Fatal(err)
//	}
package cobrashell

import "time"

// Config holds the configuration for a [Shell].
//
// All fields are optional except BinaryPath. Zero values are replaced with
// sensible defaults by [New].
type Config struct {
	// BinaryPath is the path to the Cobra binary to wrap. It may be an
	// absolute path, a relative path, or a bare name that is resolved via
	// PATH. New resolves it to an absolute path immediately using
	// exec.LookPath (bare name) or filepath.Abs (path with separator).
	BinaryPath string

	// Prompt is the string printed at the start of each input line.
	// Defaults to "> " if empty.
	Prompt string

	// HistoryFile is the path to the file used to persist command history
	// across sessions. Defaults to ~/.{basename}_history, where basename is
	// derived from filepath.Base(BinaryPath) with any extension stripped.
	HistoryFile string

	// Env contains additional environment variables, in "KEY=VALUE" form, to
	// set when invoking the binary for both command execution and tab
	// completion. They are appended to the current process environment; they
	// do not replace it. Useful for injecting credentials or forcing color
	// output (e.g. "FORCE_COLOR=1").
	Env []string

	// CompletionTimeout is the maximum time to wait for the binary to respond
	// to a __completeNoDesc request. Slow or network-backed binaries may need
	// a higher value. Defaults to 500ms.
	CompletionTimeout time.Duration

	// EnvBuiltin, when non-empty, enables a built-in command for managing
	// session-scoped environment variables. The value becomes the command
	// name (e.g. "env"). Supported subcommands: list, set KEY VALUE, unset KEY.
	//
	// The built-in is intercepted before the binary is spawned, so it works
	// even for binaries that have their own "env" subcommand — simply choose
	// a name that does not conflict.
	//
	// Defaults to "" (disabled).
	EnvBuiltin string

	// Hooks contains optional lifecycle callbacks. All fields are optional;
	// nil hooks are silently skipped.
	Hooks Hooks
}

// Hooks contains optional lifecycle callbacks for a [Shell]. All fields are
// optional; a nil function is a no-op.
type Hooks struct {
	// BeforeExec is called before each command is executed. Return a non-nil
	// error to cancel execution; the error message is printed to stderr and
	// the shell continues. Return nil to allow execution to proceed.
	//
	// Example use: validating an auth token before every command.
	BeforeExec func(args []string) error

	// AfterExec is called after each command completes, receiving the
	// tokenized arguments and the process exit code. It is called even when
	// the exit code is non-zero.
	AfterExec func(args []string, exitCode int)

	// OnStart is called once when the shell starts, before the first prompt
	// is displayed. Useful for printing a welcome banner or initialising
	// shared state.
	OnStart func(shell *Shell)

	// OnExit is called once when the shell exits cleanly via Ctrl-D or the
	// built-in "exit" command.
	OnExit func()
}
