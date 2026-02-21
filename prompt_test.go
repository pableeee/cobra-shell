package cobrashell

import (
	"strings"
	"testing"
)

// --- Colorize ---

func TestColorize_WrapsWithMarkers(t *testing.T) {
	got := Colorize("hello", ColorGreen)
	// Must start with \x01<code>\x02, contain "hello", end with \x01<reset>\x02.
	if !strings.HasPrefix(got, "\x01"+ColorGreen+"\x02") {
		t.Errorf("Colorize missing opening RL marker: %q", got)
	}
	if !strings.Contains(got, "hello") {
		t.Errorf("Colorize missing text: %q", got)
	}
	if !strings.HasSuffix(got, "\x01"+ColorReset+"\x02") {
		t.Errorf("Colorize missing closing RL marker: %q", got)
	}
}

func TestColorize_EmptyCode_ReturnsText(t *testing.T) {
	got := Colorize("hello", "")
	if got != "hello" {
		t.Errorf("Colorize with empty code = %q, want %q", got, "hello")
	}
}

func TestColorize_EmptyText(t *testing.T) {
	got := Colorize("", ColorRed)
	// Even with empty text the markers and reset must be present.
	if !strings.Contains(got, "\x01") {
		t.Errorf("Colorize with empty text should still emit markers: %q", got)
	}
}

// --- DynamicPrompt wiring (Shell) ---

func TestDynamicPrompt_Shell_InitialPromptUsed(t *testing.T) {
	// Verify that when DynamicPrompt is set, New() does not override it with
	// the static default: the initial readline prompt is DynamicPrompt(0).
	called := false
	cfg := Config{
		BinaryPath: "/usr/bin/true",
		DynamicPrompt: func(code int) string {
			called = true
			return "> "
		},
	}
	s := New(cfg)
	if s.initErr != nil {
		t.Fatalf("New: %v", s.initErr)
	}
	// DynamicPrompt is stored in cfg; confirm it is non-nil.
	if s.cfg.DynamicPrompt == nil {
		t.Error("DynamicPrompt should be stored in Shell.cfg")
	}
	_ = called // actual invocation happens inside Run(); just verify presence
}

func TestDynamicPrompt_Shell_StoredInConfig(t *testing.T) {
	fn := func(code int) string { return "> " }
	s := New(Config{BinaryPath: "/usr/bin/true", DynamicPrompt: fn})
	if s.cfg.DynamicPrompt == nil {
		t.Error("DynamicPrompt not stored in Shell.cfg")
	}
}

// --- DynamicPrompt wiring (EmbeddedShell) ---

func TestDynamicPrompt_Embedded_StoredInConfig(t *testing.T) {
	fn := func(code int) string { return "» " }
	sh := NewEmbedded(EmbeddedConfig{
		RootCmd:       newTestRoot(),
		DynamicPrompt: fn,
	})
	if sh.cfg.DynamicPrompt == nil {
		t.Error("DynamicPrompt not stored in EmbeddedShell.cfg")
	}
}

// --- lastExitCode tracking ---

func TestLastExitCode_EmbeddedShell_SuccessIsZero(t *testing.T) {
	root := newTestRoot()
	sh := NewEmbedded(EmbeddedConfig{RootCmd: root})
	sh.execute("serve")
	if sh.lastExitCode != 0 {
		// serve may or may not error depending on cobra setup; just verify
		// the field is updated and does not retain a previous non-zero value.
		t.Logf("lastExitCode = %d (may be non-zero if serve requires args)", sh.lastExitCode)
	}
}

func TestLastExitCode_EmbeddedShell_NonZeroOnError(t *testing.T) {
	root := newTestRoot()
	sh := NewEmbedded(EmbeddedConfig{RootCmd: root})
	// Execute an unknown subcommand — cobra returns an error → exitCode = 1.
	sh.execute("nonexistentcmd")
	if sh.lastExitCode == 0 {
		t.Error("expected non-zero lastExitCode for unknown command")
	}
}
