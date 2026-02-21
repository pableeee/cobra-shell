package cobrashell

import (
	"strings"

	"github.com/google/shlex"
	"github.com/spf13/pflag"
)

// embeddedCompleter implements readline.AutoCompleter for an EmbeddedShell.
// Completion is driven entirely in-process by walking the cobra command tree,
// calling ValidArgsFunction on matched commands, and consulting
// EmbeddedConfig.DynamicCompletions. No subprocess is spawned.
type embeddedCompleter struct {
	shell *EmbeddedShell
}

// Do implements readline.AutoCompleter.
func (c *embeddedCompleter) Do(line []rune, pos int) (newLine [][]rune, length int) {
	segment := string(line[:pos])

	endsWithSpace := len(segment) > 0 &&
		(segment[len(segment)-1] == ' ' || segment[len(segment)-1] == '\t')

	tokens, err := shlex.Split(segment)
	if err != nil {
		return nil, 0
	}

	var contextArgs []string
	var toComplete string

	switch {
	case endsWithSpace || len(tokens) == 0:
		contextArgs = tokens
		toComplete = ""
	default:
		contextArgs = tokens[:len(tokens)-1]
		toComplete = tokens[len(tokens)-1]
	}

	candidates := c.complete(contextArgs, toComplete)
	if len(candidates) == 0 {
		return nil, 0
	}

	prefix := []rune(toComplete)
	result := make([][]rune, len(candidates))
	for i, s := range candidates {
		result[i] = []rune(s)[len(prefix):]
	}
	return result, len(prefix)
}

// complete resolves the command addressed by contextArgs, then collects
// candidates from three sources in order:
//
//  1. Subcommand names of the matched command (when toComplete is not a flag).
//  2. EmbeddedConfig.DynamicCompletions for the matched command name.
//  3. The command's own cobra ValidArgsFunction (if registered).
//
// Flag names (--flag) are offered when toComplete starts with "-", or when
// no positional candidates were found and toComplete is empty.
func (c *embeddedCompleter) complete(contextArgs []string, toComplete string) []string {
	root := c.shell.cfg.RootCmd

	// Traverse the command tree to find the deepest matching command.
	// remaining holds the args that did not match a subcommand name.
	cmd, remaining, err := root.Traverse(contextArgs)
	if err != nil || cmd == nil {
		cmd = root
		remaining = contextArgs
	}

	var candidates []string
	wantsFlag := strings.HasPrefix(toComplete, "-")

	if !wantsFlag {
		// 1. Subcommand names.
		for _, child := range cmd.Commands() {
			if child.Hidden {
				continue
			}
			if strings.HasPrefix(child.Name(), toComplete) {
				candidates = append(candidates, child.Name())
			}
		}

		// 2. DynamicCompletions registered for this command.
		if dc, ok := c.shell.cfg.DynamicCompletions[cmd.Name()]; ok {
			candidates = append(candidates, dc(remaining, toComplete)...)
		}

		// 3. cobra's native ValidArgsFunction. cobra's own shell completion
		// applies prefix filtering after calling ValidArgsFunction, so we do
		// the same here for consistency.
		if cmd.ValidArgsFunction != nil {
			completions, directive := cmd.ValidArgsFunction(cmd, remaining, toComplete)
			if directive&compDirectiveError == 0 {
				for _, s := range completions {
					if strings.HasPrefix(s, toComplete) {
						candidates = append(candidates, s)
					}
				}
			}
		}
	}

	// Offer flag names when explicitly requested ("-" prefix) or when there
	// are no positional candidates and the partial word is empty (the user
	// tabbed after a space with no leading "-").
	if wantsFlag || (toComplete == "" && len(candidates) == 0) {
		seen := make(map[string]bool)
		addFlag := func(f *pflag.Flag) {
			if f.Hidden {
				return
			}
			name := "--" + f.Name
			if !seen[name] && strings.HasPrefix(name, toComplete) {
				candidates = append(candidates, name)
				seen[name] = true
			}
		}
		cmd.Flags().VisitAll(addFlag)
		// InheritedFlags returns persistent flags from all ancestor commands.
		cmd.InheritedFlags().VisitAll(addFlag)
	}

	return candidates
}
