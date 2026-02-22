package cobrashell

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chzyer/readline"
	"github.com/google/shlex"
)

const (
	defaultPrompt            = "> "
	defaultCompletionTimeout = 500 * time.Millisecond
)

// Shell wraps a Cobra binary in an interactive readline loop. Create one with
// [New] and start it with [Run].
type Shell struct {
	cfg          Config
	binary       string            // resolved absolute path; empty when initErr is set
	initErr      error             // deferred error from New, returned by Run
	sessionEnv   map[string]string // runtime env overrides; set via SetEnv/UnsetEnv
	lastExitCode int               // exit code of the most recently executed command
}

// New creates a Shell from cfg. BinaryPath is resolved to an absolute path
// immediately; if resolution fails the error is stored and returned by [Run].
// All zero-value Config fields are replaced with defaults before Run is called.
//
// New never returns nil.
func New(cfg Config) *Shell {
	s := &Shell{}

	binary, err := resolveBinary(cfg.BinaryPath)
	if err != nil {
		s.initErr = fmt.Errorf("cobra-shell: resolve binary %q: %w", cfg.BinaryPath, err)
		s.cfg = cfg
		return s
	}
	s.binary = binary
	s.sessionEnv = make(map[string]string)

	if cfg.Prompt == "" {
		cfg.Prompt = defaultPrompt
	}
	if cfg.CompletionTimeout == 0 {
		cfg.CompletionTimeout = defaultCompletionTimeout
	}
	if cfg.HistoryFile == "" {
		cfg.HistoryFile = defaultHistoryFilePath(binary)
	}

	s.cfg = cfg
	return s
}

// Run starts the interactive shell loop. It blocks until the user exits via
// Ctrl-D or the built-in "exit" command.
//
// Run returns a non-nil error if:
//   - BinaryPath could not be resolved (error stored by [New])
//   - readline fails to initialise (e.g. history file is unwritable)
//
// A clean exit (Ctrl-D, "exit") returns nil.
func (s *Shell) Run() error {
	if s.initErr != nil {
		return s.initErr
	}

	initialPrompt := s.cfg.Prompt
	if s.cfg.DynamicPrompt != nil {
		initialPrompt = s.cfg.DynamicPrompt(0)
	}

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          initialPrompt,
		HistoryFile:     s.cfg.HistoryFile,
		AutoComplete:    &completer{shell: s},
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
			// Ctrl-C at the prompt clears the line; readline resets the
			// display automatically — just loop back for a fresh prompt.
			continue
		}
		if err != nil {
			return fmt.Errorf("cobra-shell: readline: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			// Empty input is a no-op: no subprocess, no history entry.
			continue
		}
		if line == "exit" {
			break
		}

		s.execute(line)
		if s.cfg.DynamicPrompt != nil {
			rl.SetPrompt(s.cfg.DynamicPrompt(s.lastExitCode))
		}
	}

	if s.cfg.Hooks.OnExit != nil {
		s.cfg.Hooks.OnExit()
	}
	return nil
}

// execute tokenises line, runs BeforeExec, spawns the binary, and runs
// AfterExec. SIGINT is caught in the parent while the child runs so that
// Ctrl-C cancels the child but does not exit the shell.
func (s *Shell) execute(line string) {
	tokens, err := shlex.Split(line)
	if err != nil {
		writeErr("cobra-shell: parse error: %v\n", err)
		return
	}
	if len(tokens) == 0 {
		return
	}

	// The env built-in is handled entirely in-process; it does not invoke the
	// binary and does not trigger BeforeExec/AfterExec hooks.
	if s.handleEnvBuiltin(tokens) {
		return
	}

	if hasPipe(tokens) {
		s.executePipeline(line, tokens)
		return
	}

	if s.cfg.Hooks.BeforeExec != nil {
		if err := s.cfg.Hooks.BeforeExec(tokens); err != nil {
			writeErr("%v\n", err)
			return
		}
	}

	exitCode, err := spawnCommand(s.binary, tokens, s.buildEnv())
	if err != nil {
		writeErr("cobra-shell: %v\n", err)
	}
	s.lastExitCode = exitCode

	if s.cfg.Hooks.AfterExec != nil {
		s.cfg.Hooks.AfterExec(tokens, exitCode)
	}
}

// hasPipe reports whether any token is a standalone "|".
// shlex produces "|" as its own token only when surrounded by spaces,
// matching standard shell convention.
func hasPipe(tokens []string) bool {
	for _, t := range tokens {
		if t == "|" {
			return true
		}
	}
	return false
}

// leftOfFirstPipe returns the tokens before the first "|".
// Used to give BeforeExec/AfterExec hooks the cobra-command tokens.
func leftOfFirstPipe(tokens []string) []string {
	for i, t := range tokens {
		if t == "|" {
			return tokens[:i]
		}
	}
	return tokens
}

// executePipeline handles lines containing "|" by delegating to sh -c.
// The raw user line is passed verbatim; the binary path is single-quoted
// and prepended (s.binary is an absolute path and cannot contain a single-quote).
// BeforeExec and AfterExec receive only the left-side (cobra) tokens.
func (s *Shell) executePipeline(line string, tokens []string) {
	leftTokens := leftOfFirstPipe(tokens)

	if s.cfg.Hooks.BeforeExec != nil {
		if err := s.cfg.Hooks.BeforeExec(leftTokens); err != nil {
			writeErr("%v\n", err)
			return
		}
	}

	// Single-quote the binary path so sh treats it as a literal.
	// s.binary is always an absolute path produced by filepath.Abs or
	// exec.LookPath, which never yields a path containing a single-quote.
	script := "'" + s.binary + "' " + line

	cmd := exec.Command("sh", "-c", script)
	cmd.Env = s.buildEnv()

	exitCode, err := runPlain(cmd)
	if err != nil {
		writeErr("cobra-shell: %v\n", err)
	}
	s.lastExitCode = exitCode

	if s.cfg.Hooks.AfterExec != nil {
		s.cfg.Hooks.AfterExec(leftTokens, exitCode)
	}
}

// resolveBinary resolves path to an absolute path. Bare names (no path
// separator) are looked up via exec.LookPath; paths with a separator are
// passed through filepath.Abs.
func resolveBinary(path string) (string, error) {
	if path == "" {
		return "", errors.New("BinaryPath must not be empty")
	}
	if strings.ContainsRune(path, filepath.Separator) {
		return filepath.Abs(path)
	}
	return exec.LookPath(path)
}

// defaultHistoryFilePath returns ~/.{basename}_history for the given resolved
// binary path. Errors from os.UserHomeDir are silently ignored; readline
// handles an empty HistoryFile gracefully (no persistence).
func defaultHistoryFilePath(binary string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	base := filepath.Base(binary)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	return filepath.Join(home, "."+base+"_history")
}
