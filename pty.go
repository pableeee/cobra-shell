package cobrashell

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/term"
)

// spawnCommand runs binary with tokens, using a PTY when stdin is a real
// terminal and falling back to a plain subprocess otherwise.
//
// PTY mode enables colour output for binaries that check isatty, and allows
// interactive subcommands (vim, less, ssh) to work correctly. When stdin is
// not a terminal (tests, pipelines) or PTY creation fails, plain mode is used
// with direct stdin/stdout/stderr inheritance.
func spawnCommand(binary string, tokens []string, env []string) (exitCode int, err error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		cmd := exec.Command(binary, tokens...)
		cmd.Env = env
		// pty.Start sets cmd.Stdin/Stdout/Stderr to the slave end and calls
		// cmd.Start. If it returns an error, cmd has not been started, so we
		// can safely fall through to runPlain with a fresh exec.Cmd.
		if ptmx, ptErr := pty.Start(cmd); ptErr == nil {
			return runWithPTY(cmd, ptmx)
		}
	}

	cmd := exec.Command(binary, tokens...)
	cmd.Env = env
	return runPlain(cmd)
}

// runWithPTY drives an already-started subprocess through its PTY master.
//
// Signal model: term.MakeRaw disables ISIG on the real terminal, so Ctrl-C
// produces byte 0x03 rather than a signal. That byte is forwarded to the PTY
// master; the slave's line discipline converts it to SIGINT for the subprocess
// process group. The cobra-shell parent process never receives SIGINT while in
// raw mode, so no explicit SIGINT suppression is needed here.
func runWithPTY(cmd *exec.Cmd, ptmx *os.File) (exitCode int, err error) {
	defer func() { _ = ptmx.Close() }()

	// Propagate terminal size changes to the PTY so the subprocess sees the
	// correct dimensions (especially important for pagers and TUIs).
	winchC := make(chan os.Signal, 1)
	signal.Notify(winchC, syscall.SIGWINCH)
	defer func() {
		signal.Stop(winchC)
		close(winchC)
	}()
	go func() {
		for range winchC {
			_ = pty.InheritSize(os.Stdin, ptmx)
		}
	}()
	// Set initial size before the subprocess draws anything.
	winchC <- syscall.SIGWINCH

	// Put the real terminal in raw mode. All bytes (including Ctrl-C) are
	// forwarded verbatim to the PTY master; the slave processes them.
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		// Should not happen after IsTerminal check, but handle gracefully.
		_ = cmd.Wait()
		return 0, fmt.Errorf("cobra-shell: set raw mode: %w", err)
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	// Bidirectional copy: real terminal ↔ PTY master.
	// The stdin→ptmx goroutine exits when ptmx is closed.
	go func() { _, _ = io.Copy(ptmx, os.Stdin) }()
	// ptmx→stdout returns with EIO when the slave is closed (subprocess exits).
	_, _ = io.Copy(os.Stdout, ptmx)

	if err := cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return 0, err
	}
	return 0, nil
}

// runPlain runs cmd with inherited stdin/stdout/stderr and no PTY.
// SIGINT is suppressed in the parent while the child runs: the terminal
// delivers SIGINT to the entire foreground process group, so the child
// still receives it and can handle or be killed by it normally.
func runPlain(cmd *exec.Cmd) (exitCode int, err error) {
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return 0, err
	}
	return 0, nil
}
