package cobrashell

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/google/shlex"
)

// ShellCompDirective bitmask values, as defined by cobra (1 << iota from 1).
const (
	compDirectiveError      = 1  // Completion failed; suppress results.
	compDirectiveNoSpace    = 2  // Do not append a space after the completion. (unused by readline)
	compDirectiveNoFileComp = 4  // Suppress file completion fallback.
)

// completer implements readline.AutoCompleter by invoking the wrapped binary's
// __completeNoDesc command on every Tab press.
type completer struct {
	shell *Shell
}

// Do implements readline.AutoCompleter. readline calls it with the full current
// line and the cursor position whenever the user presses Tab.
//
// It returns the list of completion candidates (each replacing the last
// `length` runes before the cursor) and the number of runes to replace.
func (c *completer) Do(line []rune, pos int) (newLine [][]rune, length int) {
	// Work only with the portion of the line up to the cursor.
	segment := string(line[:pos])

	// Detect whether the segment ends with whitespace so we know whether the
	// partial word is empty (user tabbed after a space) or non-empty.
	endsWithSpace := len(segment) > 0 &&
		(segment[len(segment)-1] == ' ' || segment[len(segment)-1] == '\t')

	tokens, err := shlex.Split(segment)
	if err != nil {
		// Unclosed quote or other parse error — no completions.
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

	candidates, directive := c.complete(contextArgs, toComplete)
	if directive&compDirectiveError != 0 || len(candidates) == 0 {
		return nil, 0
	}

	result := make([][]rune, len(candidates))
	for i, s := range candidates {
		result[i] = []rune(s)
	}
	return result, len([]rune(toComplete))
}

// complete tries __completeNoDesc first. If the binary does not support it
// (non-zero exit), it falls back to --help parsing via helpFallback.
func (c *completer) complete(contextArgs []string, toComplete string) ([]string, int) {
	candidates, directive, ok := c.tryComplete(contextArgs, toComplete)
	if ok {
		return candidates, directive
	}
	return c.helpFallback(contextArgs, toComplete)
}

// tryComplete invokes __completeNoDesc and parses the result.
// ok is false when the binary exits non-zero, indicating it does not support
// __completeNoDesc; in that case the caller should try the --help fallback.
func (c *completer) tryComplete(contextArgs []string, toComplete string) (candidates []string, directive int, ok bool) {
	args := make([]string, 0, 1+len(contextArgs)+1)
	args = append(args, "__completeNoDesc")
	args = append(args, contextArgs...)
	args = append(args, toComplete)

	ctx, cancel := context.WithTimeout(context.Background(), c.shell.cfg.CompletionTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.shell.binary, args...)
	cmd.Env = append(os.Environ(), c.shell.cfg.Env...)
	cmd.Stderr = io.Discard

	var buf bytes.Buffer
	cmd.Stdout = &buf

	if err := cmd.Run(); err != nil {
		// Non-zero exit: binary does not support __completeNoDesc.
		return nil, 0, false
	}

	candidates, directive = parseCompletions(buf.String())
	return candidates, directive, true
}

// parseCompletions parses the stdout of a __completeNoDesc invocation.
//
// Format:
//
//	candidate1
//	candidate2
//	:N
//
// where N is the ShellCompDirective bitmask. Lines before the directive line
// are the completion candidates; the directive line always starts with ':'.
// Scanning from the end makes the parser robust against binaries that emit
// extra output before the candidates.
func parseCompletions(output string) (candidates []string, directive int) {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	// Scan from the end for the directive line.
	for i := len(lines) - 1; i >= 0; i-- {
		if len(lines[i]) == 0 || lines[i][0] != ':' {
			continue
		}
		n, err := strconv.Atoi(lines[i][1:])
		if err != nil {
			continue
		}
		directive = n
		for _, c := range lines[:i] {
			if c != "" {
				candidates = append(candidates, c)
			}
		}
		return
	}

	// No directive line found — binary may not support __completeNoDesc.
	return nil, 0
}
