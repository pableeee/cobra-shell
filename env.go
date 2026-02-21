package cobrashell

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// SetEnv sets a session-scoped environment variable. The value takes effect on
// the next command execution or tab-completion request; it does not modify the
// current process environment (os.Setenv is never called). If called multiple
// times with the same key, the last value wins.
//
// Session env variables take precedence over both os.Environ() and Config.Env
// when building the subprocess environment.
func (s *Shell) SetEnv(key, value string) {
	s.sessionEnv[key] = value
}

// UnsetEnv removes a session-scoped environment variable previously set via
// [Shell.SetEnv]. If key is not present it is a no-op.
func (s *Shell) UnsetEnv(key string) {
	delete(s.sessionEnv, key)
}

// SessionEnv returns a snapshot of the current session environment as a sorted
// slice of "KEY=VALUE" strings. It does not include os.Environ() or Config.Env.
func (s *Shell) SessionEnv() []string {
	pairs := make([]string, 0, len(s.sessionEnv))
	for k, v := range s.sessionEnv {
		pairs = append(pairs, k+"="+v)
	}
	sort.Strings(pairs)
	return pairs
}

// buildEnv constructs the subprocess environment by merging three sources in
// ascending priority order:
//
//  1. os.Environ() — the shell process's inherited environment.
//  2. Config.Env — static additive variables from configuration.
//  3. sessionEnv — variables set at runtime via [Shell.SetEnv].
//
// Later values for the same key shadow earlier ones because os/exec uses the
// last occurrence in Cmd.Env. os.Setenv is never called.
func (s *Shell) buildEnv() []string {
	env := append(os.Environ(), s.cfg.Env...)
	for k, v := range s.sessionEnv {
		env = append(env, k+"="+v)
	}
	return env
}

// handleEnvBuiltin checks whether tokens[0] matches Config.EnvBuiltin. If so,
// it processes the built-in env command and returns true. If EnvBuiltin is
// empty or the first token does not match, it returns false and the caller
// should proceed with normal execution.
//
// Supported subcommands: list, set KEY VALUE, unset KEY.
func (s *Shell) handleEnvBuiltin(tokens []string) bool {
	if s.cfg.EnvBuiltin == "" || tokens[0] != s.cfg.EnvBuiltin {
		return false
	}
	if len(tokens) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s {list|set KEY VALUE|unset KEY}\n", s.cfg.EnvBuiltin)
		return true
	}
	switch tokens[1] {
	case "list":
		for _, pair := range s.SessionEnv() {
			fmt.Println(pair)
		}
	case "set":
		if len(tokens) != 4 {
			fmt.Fprintf(os.Stderr, "usage: %s set KEY VALUE\n", s.cfg.EnvBuiltin)
			return true
		}
		s.SetEnv(tokens[2], tokens[3])
	case "unset":
		if len(tokens) != 3 {
			fmt.Fprintf(os.Stderr, "usage: %s unset KEY\n", s.cfg.EnvBuiltin)
			return true
		}
		s.UnsetEnv(tokens[2])
	default:
		fmt.Fprintf(os.Stderr, "cobra-shell: %s: unknown subcommand %q\n", s.cfg.EnvBuiltin, tokens[1])
	}
	return true
}

// envBuiltinKeys extracts the KEY part from each "KEY=VALUE" pair returned by
// [Shell.SessionEnv]. Used by the completer to offer unset candidates.
func envBuiltinKeys(pairs []string) []string {
	keys := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		if k, _, ok := strings.Cut(pair, "="); ok {
			keys = append(keys, k)
		}
	}
	return keys
}
