package cobrashell

import (
	"testing"

	"github.com/spf13/cobra"
)

// --- NewEmbedded / Run ---

func TestNewEmbedded_NilRootCmd(t *testing.T) {
	sh := NewEmbedded(EmbeddedConfig{})
	if err := sh.Run(); err == nil {
		t.Fatal("expected error for nil RootCmd, got nil")
	}
}

func TestNewEmbedded_Defaults(t *testing.T) {
	root := &cobra.Command{Use: "myapp"}
	sh := NewEmbedded(EmbeddedConfig{RootCmd: root})

	if sh.cfg.Prompt != defaultPrompt {
		t.Errorf("Prompt = %q, want %q", sh.cfg.Prompt, defaultPrompt)
	}
	if sh.cfg.CompletionTimeout != defaultCompletionTimeout {
		t.Errorf("CompletionTimeout = %v, want %v", sh.cfg.CompletionTimeout, defaultCompletionTimeout)
	}
	if sh.cfg.HistoryFile == "" {
		t.Error("HistoryFile should have a default value")
	}
}

// --- resetCommandTree ---

func TestResetCommandTree_ResetsChangedFlag(t *testing.T) {
	var port int
	cmd := &cobra.Command{Use: "serve"}
	cmd.Flags().IntVar(&port, "port", 8080, "Port")

	// Simulate a previous execution that set --port 9090.
	if err := cmd.Flags().Set("port", "9090"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if port != 9090 {
		t.Fatalf("expected port 9090 after Set, got %d", port)
	}

	resetCommandTree(cmd)

	if port != 8080 {
		t.Fatalf("expected port 8080 after reset, got %d", port)
	}
	f := cmd.Flags().Lookup("port")
	if f.Changed {
		t.Fatal("expected f.Changed false after reset")
	}
}

func TestResetCommandTree_Recursive(t *testing.T) {
	var childVal string
	root := &cobra.Command{Use: "root"}
	child := &cobra.Command{Use: "child"}
	child.Flags().StringVar(&childVal, "name", "default", "Name")
	root.AddCommand(child)

	_ = child.Flags().Set("name", "custom")
	if childVal != "custom" {
		t.Fatalf("expected 'custom' after Set, got %q", childVal)
	}

	resetCommandTree(root)

	if childVal != "default" {
		t.Fatalf("expected 'default' after reset, got %q", childVal)
	}
}

func TestResetCommandTree_PersistentFlags(t *testing.T) {
	var verbose bool
	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().BoolVar(&verbose, "verbose", false, "Verbose")

	_ = root.PersistentFlags().Set("verbose", "true")
	if !verbose {
		t.Fatal("expected verbose true after Set")
	}

	resetCommandTree(root)

	if verbose {
		t.Fatal("expected verbose false after reset")
	}
}

// --- embeddedCompleter ---

func newTestRoot() *cobra.Command {
	root := &cobra.Command{Use: "myapp"}
	root.PersistentFlags().Bool("verbose", false, "Enable verbose output")

	serve := &cobra.Command{Use: "serve", Short: "Start the server"}
	serve.Flags().Int("port", 8080, "Port")
	root.AddCommand(serve)

	root.AddCommand(&cobra.Command{Use: "version", Short: "Print version"})
	root.AddCommand(&cobra.Command{Use: "help-me", Short: "Help", Hidden: true})

	return root
}

func TestEmbeddedCompleter_Subcommands(t *testing.T) {
	sh := NewEmbedded(EmbeddedConfig{RootCmd: newTestRoot()})
	c := &embeddedCompleter{shell: sh}

	got := c.complete(nil, "se")
	if len(got) != 1 || got[0] != "serve" {
		t.Errorf("complete(nil, 'se') = %v, want [serve]", got)
	}
}

func TestEmbeddedCompleter_AllSubcommands(t *testing.T) {
	sh := NewEmbedded(EmbeddedConfig{RootCmd: newTestRoot()})
	c := &embeddedCompleter{shell: sh}

	got := c.complete(nil, "")
	// help-me is Hidden so should not appear; serve and version should.
	names := make(map[string]bool)
	for _, g := range got {
		names[g] = true
	}
	if !names["serve"] {
		t.Error("expected 'serve' in candidates")
	}
	if !names["version"] {
		t.Error("expected 'version' in candidates")
	}
	if names["help-me"] {
		t.Error("hidden command 'help-me' should not appear in candidates")
	}
}

func TestEmbeddedCompleter_FlagPrefix(t *testing.T) {
	sh := NewEmbedded(EmbeddedConfig{RootCmd: newTestRoot()})
	c := &embeddedCompleter{shell: sh}

	// At the serve subcommand level, "--p" should match "--port".
	got := c.complete([]string{"serve"}, "--p")
	found := false
	for _, g := range got {
		if g == "--port" {
			found = true
		}
	}
	if !found {
		t.Errorf("complete(['serve'], '--p') = %v, want --port among results", got)
	}
}

func TestEmbeddedCompleter_PersistentFlag(t *testing.T) {
	sh := NewEmbedded(EmbeddedConfig{RootCmd: newTestRoot()})
	c := &embeddedCompleter{shell: sh}

	// --verbose is a persistent flag on root; it should appear under serve.
	got := c.complete([]string{"serve"}, "--v")
	found := false
	for _, g := range got {
		if g == "--verbose" {
			found = true
		}
	}
	if !found {
		t.Errorf("complete(['serve'], '--v') = %v, want --verbose among results", got)
	}
}

func TestEmbeddedCompleter_DynamicCompletions(t *testing.T) {
	root := newTestRoot()
	sh := NewEmbedded(EmbeddedConfig{
		RootCmd: root,
		DynamicCompletions: map[string]CompletionFunc{
			"serve": func(args []string, toComplete string) []string {
				return []string{"profile1", "profile2"}
			},
		},
	})
	c := &embeddedCompleter{shell: sh}

	got := c.complete([]string{"serve"}, "pro")
	if len(got) != 2 {
		t.Errorf("expected 2 dynamic candidates, got %v", got)
	}
}

// TestEmbeddedCompleter_Do_ReturnsSuffix guards against the readline contract:
// Do() must return suffixes (the part after the typed prefix), because readline
// calls buf.WriteRunes(candidate) which appends without removing anything.
// Returning full words causes doubling: typing "se" + Tab would give "seserve".
func TestEmbeddedCompleter_Do_ReturnsSuffix(t *testing.T) {
	sh := NewEmbedded(EmbeddedConfig{RootCmd: newTestRoot()})
	c := &embeddedCompleter{shell: sh}

	line := []rune("se")
	candidates, length := c.Do(line, len(line))

	if length != 2 {
		t.Errorf("length = %d, want 2 (len of 'se')", length)
	}
	if len(candidates) == 0 {
		t.Fatal("expected candidates, got none")
	}
	for _, cand := range candidates {
		if string(cand) == "serve" {
			t.Errorf("Do returned full word %q; want suffix %q", "serve", "rve")
		}
	}
	found := false
	for _, cand := range candidates {
		if string(cand) == "rve" {
			found = true
		}
	}
	if !found {
		t.Errorf("suffix 'rve' not found in candidates %v", candidates)
	}
}

func TestEmbeddedCompleter_Do_EmptyPrefix_ReturnsFullWord(t *testing.T) {
	// When toComplete is empty (user tabbed after a space), the suffix equals
	// the full word — verify no rune-slicing panic or truncation.
	sh := NewEmbedded(EmbeddedConfig{RootCmd: newTestRoot()})
	c := &embeddedCompleter{shell: sh}

	line := []rune("")
	candidates, length := c.Do(line, 0)

	if length != 0 {
		t.Errorf("length = %d, want 0 for empty prefix", length)
	}
	names := make(map[string]bool)
	for _, cand := range candidates {
		names[string(cand)] = true
	}
	if !names["serve"] || !names["version"] {
		t.Errorf("expected full names when prefix is empty, got %v", candidates)
	}
}

func TestEmbeddedCompleter_ValidArgsFunction(t *testing.T) {
	root := &cobra.Command{Use: "myapp"}
	sub := &cobra.Command{
		Use: "get",
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return []string{"alpha", "bravo", "charlie"}, cobra.ShellCompDirectiveNoFileComp
		},
	}
	root.AddCommand(sub)

	sh := NewEmbedded(EmbeddedConfig{RootCmd: root})
	c := &embeddedCompleter{shell: sh}

	got := c.complete([]string{"get"}, "a")
	if len(got) != 1 || got[0] != "alpha" {
		t.Errorf("complete(['get'], 'a') = %v, want [alpha]", got)
	}
}
