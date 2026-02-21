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
	name := s.cfg.EnvBuiltin
	if name == "" || tokens[0] != name {
		return false
	}

	// No subcommand or top-level --help / -h.
	if len(tokens) < 2 || tokens[1] == "--help" || tokens[1] == "-h" {
		fmt.Printf("Manage session-scoped environment variables.\n\n"+
			"Usage:\n  %s [command]\n\n"+
			"Available Commands:\n"+
			"  list        List all session environment variables\n"+
			"  set         Set a session environment variable\n"+
			"  unset       Remove a session environment variable\n\n"+
			"Use \"%s [command] --help\" for more information about a command.\n",
			name, name)
		return true
	}

	sub := tokens[1]
	rest := tokens[2:]

	// Check for --help / -h on subcommands before dispatching.
	wantsHelp := len(rest) > 0 && (rest[0] == "--help" || rest[0] == "-h")

	switch sub {
	case "list":
		if wantsHelp {
			fmt.Printf("List all session environment variables.\n\n"+
				"Usage:\n  %s list\n", name)
			return true
		}
		for _, pair := range s.SessionEnv() {
			fmt.Println(pair)
		}

	case "set":
		if wantsHelp {
			fmt.Printf("Set a session environment variable.\n"+
				"The value takes effect on the next command execution.\n\n"+
				"Usage:\n  %s set KEY VALUE\n", name)
			return true
		}
		if len(tokens) != 4 {
			writeErr("Error: accepts 2 args, received %d\n\nUsage:\n  %s set KEY VALUE\n",
				len(rest), name)
			return true
		}
		s.SetEnv(tokens[2], tokens[3])

	case "unset":
		if wantsHelp {
			fmt.Printf("Remove a session environment variable.\n\n"+
				"Usage:\n  %s unset KEY\n", name)
			return true
		}
		if len(tokens) != 3 {
			writeErr("Error: accepts 1 arg, received %d\n\nUsage:\n  %s unset KEY\n",
				len(rest), name)
			return true
		}
		s.UnsetEnv(tokens[2])

	default:
		writeErr("Error: unknown command %q for %q\nRun '%s --help' for usage.\n",
			sub, name, name)
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
