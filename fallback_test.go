package cobrashell

import (
	"strings"
	"testing"
)

// cobraHelp is a representative sample of Cobra's default --help output.
const cobraHelp = `An application for doing useful things.

Usage:
  myapp [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  serve       Start the HTTP server
  version     Print the version number

Flags:
  -h, --help          help for myapp
  -p, --port int      Port to listen on (default 8080)
      --verbose       Enable verbose output
  -c, --config string Path to config file

Global Flags:
      --debug   Enable debug mode

Use "myapp [command] --help" for more information about a command.
`

func TestParseHelp_AllCandidates(t *testing.T) {
	got := parseHelp(cobraHelp, "")
	want := []string{
		"completion", "help", "serve", "version",
		"--help", "--port", "--verbose", "--config",
		"--debug",
	}
	assertSameElements(t, got, want)
}

func TestParseHelp_CommandPrefix(t *testing.T) {
	got := parseHelp(cobraHelp, "se")
	if len(got) != 1 || got[0] != "serve" {
		t.Errorf("got %v, want [serve]", got)
	}
}

func TestParseHelp_FlagPrefix(t *testing.T) {
	got := parseHelp(cobraHelp, "--p")
	if len(got) != 1 || got[0] != "--port" {
		t.Errorf("got %v, want [--port]", got)
	}
}

func TestParseHelp_AllFlags(t *testing.T) {
	got := parseHelp(cobraHelp, "--")
	wantFlags := []string{"--help", "--port", "--verbose", "--config", "--debug"}
	assertSameElements(t, got, wantFlags)
}

func TestParseHelp_NoMatch(t *testing.T) {
	got := parseHelp(cobraHelp, "xyz")
	if len(got) != 0 {
		t.Errorf("expected no candidates, got %v", got)
	}
}

func TestParseHelp_EmptyOutput(t *testing.T) {
	got := parseHelp("", "")
	if len(got) != 0 {
		t.Errorf("expected no candidates for empty output, got %v", got)
	}
}

func TestParseHelp_NonCobra(t *testing.T) {
	// Arbitrary non-Cobra help text should not panic and should return empty.
	got := parseHelp("usage: sometool [-h] [--foo BAR]\n\nOptions:\n  -h  show help\n", "")
	// We don't assert specific results — just that it doesn't panic and
	// doesn't return obvious garbage (no leading "--" from prose).
	for _, c := range got {
		if strings.Contains(c, " ") {
			t.Errorf("candidate %q contains a space — likely a parse error", c)
		}
	}
}

func TestParseHelp_CommandsAlias(t *testing.T) {
	// Some Cobra templates use "Commands:" instead of "Available Commands:".
	help := `Usage:
  tool [command]

Commands:
  foo  Does foo
  bar  Does bar

Flags:
  -h, --help  help for tool
`
	got := parseHelp(help, "")
	assertSameElements(t, got, []string{"foo", "bar", "--help"})
}

// assertSameElements checks that got contains exactly the elements in want,
// regardless of order.
func assertSameElements(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("length mismatch: got %d %v, want %d %v", len(got), got, len(want), want)
		return
	}
	index := make(map[string]int, len(want))
	for _, w := range want {
		index[w]++
	}
	for _, g := range got {
		index[g]--
		if index[g] < 0 {
			t.Errorf("unexpected candidate %q", g)
		}
	}
	for k, v := range index {
		if v != 0 {
			t.Errorf("candidate %q missing from result", k)
		}
	}
}
