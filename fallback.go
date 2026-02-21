package cobrashell

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
)

// helpFallback derives completions by running `binary [contextArgs...] --help`
// and parsing the output. It is used when __completeNoDesc is unavailable
// (pre-Cobra binary, non-Cobra binary, or Cobra < 1.2).
//
// The returned directive is always 0 (Default); there is no directive line in
// --help output to parse.
func (c *completer) helpFallback(contextArgs []string, toComplete string) ([]string, int) {
	args := append(contextArgs, "--help")

	ctx, cancel := context.WithTimeout(context.Background(), c.shell.cfg.CompletionTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.shell.binary, args...)
	cmd.Env = append(os.Environ(), c.shell.cfg.Env...)
	cmd.Stderr = io.Discard

	// Some binaries print help to stdout with exit code 0; others exit non-zero.
	// Capture stdout unconditionally and ignore the exit code.
	var buf bytes.Buffer
	cmd.Stdout = &buf
	_ = cmd.Run()

	return parseHelp(buf.String(), toComplete), 0
}

// parseHelp extracts completion candidates from Cobra's --help output.
//
// It recognises the standard Cobra help template section headers:
//   - "Available Commands:" / "Commands:" → yields subcommand names
//   - "Flags:" / "Global Flags:"          → yields --flag-name tokens
//
// Parsing is heuristic: it handles the default Cobra template reliably but
// may produce incomplete results for heavily customised templates. It cannot
// complete flag values — only flag names.
//
// Unlike __completeNoDesc, --help returns all entries regardless of prefix, so
// parseHelp performs client-side prefix filtering against toComplete.
func parseHelp(output, toComplete string) []string {
	type section int
	const (
		secNone     section = iota
		secCommands         // inside "Available Commands:" / "Commands:"
		secFlags            // inside "Flags:" / "Global Flags:"
	)

	var candidates []string
	cur := secNone

	for _, line := range strings.Split(output, "\n") {
		// A blank line ends the current section.
		if strings.TrimSpace(line) == "" {
			cur = secNone
			continue
		}

		stripped := strings.TrimLeft(line, " \t")
		indent := len(line) - len(stripped)

		if indent == 0 {
			// Unindented lines are section headers or other prose.
			switch {
			case strings.HasPrefix(stripped, "Available Commands:"),
				strings.HasPrefix(stripped, "Commands:"):
				cur = secCommands
			case strings.HasPrefix(stripped, "Flags:"),
				strings.HasPrefix(stripped, "Global Flags:"):
				cur = secFlags
			default:
				cur = secNone
			}
			continue
		}

		// Indented content lines.
		switch cur {
		case secCommands:
			// Format: "  subcommand   Short description"
			if fields := strings.Fields(stripped); len(fields) > 0 {
				candidates = append(candidates, fields[0])
			}

		case secFlags:
			// Format: "  -s, --longflag type   Description"
			//      or "      --longflag type   Description"
			// Grab the first token that starts with "--".
			for _, field := range strings.Fields(stripped) {
				f := strings.TrimRight(field, ",")
				if strings.HasPrefix(f, "--") {
					candidates = append(candidates, f)
					break
				}
			}
		}
	}

	// Client-side prefix filtering.
	if toComplete == "" {
		return candidates
	}
	var filtered []string
	for _, c := range candidates {
		if strings.HasPrefix(c, toComplete) {
			filtered = append(filtered, c)
		}
	}
	return filtered
}
