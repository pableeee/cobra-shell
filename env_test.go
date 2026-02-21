package cobrashell

import (
	"strings"
	"testing"
)

// makeEnvShell builds a Shell with sessionEnv initialised, bypassing binary
// resolution so that tests that don't spawn subprocesses can run anywhere.
func makeEnvShell(envBuiltin string) *Shell {
	return &Shell{
		cfg:        Config{EnvBuiltin: envBuiltin},
		binary:     "/usr/bin/true",
		sessionEnv: make(map[string]string),
	}
}

// --- SetEnv / UnsetEnv / SessionEnv ---

func TestSetEnv_StoresValue(t *testing.T) {
	s := makeEnvShell("")
	s.SetEnv("FOO", "bar")
	if got := s.sessionEnv["FOO"]; got != "bar" {
		t.Errorf("sessionEnv[FOO] = %q, want %q", got, "bar")
	}
}

func TestSetEnv_Overwrite(t *testing.T) {
	s := makeEnvShell("")
	s.SetEnv("FOO", "bar")
	s.SetEnv("FOO", "baz")
	if got := s.sessionEnv["FOO"]; got != "baz" {
		t.Errorf("after overwrite, sessionEnv[FOO] = %q, want %q", got, "baz")
	}
}

func TestUnsetEnv_Removes(t *testing.T) {
	s := makeEnvShell("")
	s.SetEnv("FOO", "bar")
	s.UnsetEnv("FOO")
	if _, ok := s.sessionEnv["FOO"]; ok {
		t.Error("expected FOO to be deleted after UnsetEnv")
	}
}

func TestUnsetEnv_MissingKeyIsNoOp(t *testing.T) {
	s := makeEnvShell("")
	// Must not panic.
	s.UnsetEnv("MISSING")
}

func TestSessionEnv_SortedPairs(t *testing.T) {
	s := makeEnvShell("")
	s.SetEnv("ZZZ", "last")
	s.SetEnv("AAA", "first")
	s.SetEnv("MMM", "mid")
	got := s.SessionEnv()
	if len(got) != 3 {
		t.Fatalf("SessionEnv() length = %d, want 3", len(got))
	}
	if got[0] != "AAA=first" || got[1] != "MMM=mid" || got[2] != "ZZZ=last" {
		t.Errorf("SessionEnv() = %v, wrong order or values", got)
	}
}

func TestSessionEnv_Empty(t *testing.T) {
	s := makeEnvShell("")
	if got := s.SessionEnv(); len(got) != 0 {
		t.Errorf("SessionEnv() on empty store = %v, want []", got)
	}
}

// --- buildEnv ---

func TestBuildEnv_SessionShadowsConfig(t *testing.T) {
	s := &Shell{
		cfg:        Config{Env: []string{"PRIORITY=config"}},
		binary:     "/usr/bin/true",
		sessionEnv: map[string]string{"PRIORITY": "session"},
	}
	env := s.buildEnv()
	// os/exec uses the last occurrence, so find the last PRIORITY entry.
	last := ""
	for _, e := range env {
		if strings.HasPrefix(e, "PRIORITY=") {
			last = e
		}
	}
	if last != "PRIORITY=session" {
		t.Errorf("last PRIORITY entry = %q, want %q", last, "PRIORITY=session")
	}
}

func TestBuildEnv_InheritsOsEnv(t *testing.T) {
	t.Setenv("CS_INHERIT_TEST", "yes")
	s := &Shell{
		cfg:        Config{},
		binary:     "/usr/bin/true",
		sessionEnv: make(map[string]string),
	}
	found := false
	for _, e := range s.buildEnv() {
		if e == "CS_INHERIT_TEST=yes" {
			found = true
		}
	}
	if !found {
		t.Error("os environ key not present in buildEnv output")
	}
}

func TestBuildEnv_ConfigEnvPresent(t *testing.T) {
	s := &Shell{
		cfg:        Config{Env: []string{"STATIC=1"}},
		binary:     "/usr/bin/true",
		sessionEnv: make(map[string]string),
	}
	found := false
	for _, e := range s.buildEnv() {
		if e == "STATIC=1" {
			found = true
		}
	}
	if !found {
		t.Error("Config.Env entry not present in buildEnv output")
	}
}

// --- handleEnvBuiltin ---

func TestHandleEnvBuiltin_Disabled(t *testing.T) {
	s := makeEnvShell("")
	if s.handleEnvBuiltin([]string{"env"}) {
		t.Error("handleEnvBuiltin with empty EnvBuiltin should return false")
	}
}

func TestHandleEnvBuiltin_WrongName(t *testing.T) {
	s := makeEnvShell("environ")
	if s.handleEnvBuiltin([]string{"env"}) {
		t.Error("handleEnvBuiltin with non-matching name should return false")
	}
}

func TestHandleEnvBuiltin_NoSubcommand(t *testing.T) {
	s := makeEnvShell("env")
	// Prints usage to stderr; just verify it returns true without panicking.
	if !s.handleEnvBuiltin([]string{"env"}) {
		t.Error("handleEnvBuiltin with no subcommand should return true")
	}
}

func TestHandleEnvBuiltin_List(t *testing.T) {
	s := makeEnvShell("env")
	s.SetEnv("K", "V")
	if !s.handleEnvBuiltin([]string{"env", "list"}) {
		t.Error("handleEnvBuiltin list should return true")
	}
}

func TestHandleEnvBuiltin_Set(t *testing.T) {
	s := makeEnvShell("env")
	if !s.handleEnvBuiltin([]string{"env", "set", "MY_KEY", "my_value"}) {
		t.Error("handleEnvBuiltin set should return true")
	}
	if s.sessionEnv["MY_KEY"] != "my_value" {
		t.Errorf("expected MY_KEY=my_value in sessionEnv, got %v", s.sessionEnv)
	}
}

func TestHandleEnvBuiltin_SetWrongArgCount(t *testing.T) {
	s := makeEnvShell("env")
	// Too few args: prints usage, returns true.
	if !s.handleEnvBuiltin([]string{"env", "set", "ONLY_KEY"}) {
		t.Error("handleEnvBuiltin set with wrong arg count should return true")
	}
}

func TestHandleEnvBuiltin_Unset(t *testing.T) {
	s := makeEnvShell("env")
	s.SetEnv("DEL", "me")
	if !s.handleEnvBuiltin([]string{"env", "unset", "DEL"}) {
		t.Error("handleEnvBuiltin unset should return true")
	}
	if _, ok := s.sessionEnv["DEL"]; ok {
		t.Error("key should have been deleted by unset")
	}
}

func TestHandleEnvBuiltin_UnsetWrongArgCount(t *testing.T) {
	s := makeEnvShell("env")
	if !s.handleEnvBuiltin([]string{"env", "unset"}) {
		t.Error("handleEnvBuiltin unset with no KEY should return true")
	}
}

func TestHandleEnvBuiltin_UnknownSubcommand(t *testing.T) {
	s := makeEnvShell("env")
	// Prints error to stderr, returns true.
	if !s.handleEnvBuiltin([]string{"env", "badcmd"}) {
		t.Error("handleEnvBuiltin with unknown subcommand should return true")
	}
}

// --- completer.doEnvBuiltin ---

func makeEnvCompleter(envBuiltin string) *completer {
	s := &Shell{
		cfg: Config{
			EnvBuiltin:        envBuiltin,
			CompletionTimeout: defaultCompletionTimeout,
		},
		binary:     "/usr/bin/true",
		sessionEnv: map[string]string{"ALPHA": "1", "BETA": "2"},
	}
	return &completer{shell: s}
}

func TestDoEnvBuiltin_AllSubcommands(t *testing.T) {
	c := makeEnvCompleter("env")
	got, length := c.doEnvBuiltin(nil, "")
	if len(got) != 3 {
		t.Errorf("expected 3 subcommand candidates, got %d: %v", len(got), got)
	}
	if length != 0 {
		t.Errorf("expected length 0 for empty toComplete, got %d", length)
	}
}

func TestDoEnvBuiltin_SubcommandPrefix(t *testing.T) {
	c := makeEnvCompleter("env")
	got, _ := c.doEnvBuiltin(nil, "s")
	// doEnvBuiltin returns suffixes: "set"[1:] = "et"
	if len(got) != 1 || string(got[0]) != "et" {
		t.Errorf("expected suffix [et] for prefix 's', got %v", got)
	}
}

func TestDoEnvBuiltin_NoMatchSubcommand(t *testing.T) {
	c := makeEnvCompleter("env")
	got, _ := c.doEnvBuiltin(nil, "z")
	if len(got) != 0 {
		t.Errorf("expected no candidates for prefix 'z', got %v", got)
	}
}

func TestDoEnvBuiltin_UnsetAllKeys(t *testing.T) {
	c := makeEnvCompleter("env")
	got, _ := c.doEnvBuiltin([]string{"unset"}, "")
	if len(got) != 2 {
		t.Errorf("expected 2 key candidates for 'unset', got %v", got)
	}
}

func TestDoEnvBuiltin_UnsetKeyPrefix(t *testing.T) {
	c := makeEnvCompleter("env")
	got, _ := c.doEnvBuiltin([]string{"unset"}, "A")
	// doEnvBuiltin returns suffixes: "ALPHA"[1:] = "LPHA"
	if len(got) != 1 || string(got[0]) != "LPHA" {
		t.Errorf("expected suffix [LPHA] for prefix 'A', got %v", got)
	}
}

func TestDoEnvBuiltin_SetNoCompletion(t *testing.T) {
	c := makeEnvCompleter("env")
	got, _ := c.doEnvBuiltin([]string{"set"}, "")
	if len(got) != 0 {
		t.Errorf("expected no candidates for 'set KEY', got %v", got)
	}
}

func TestDoEnvBuiltin_ListNoCompletion(t *testing.T) {
	c := makeEnvCompleter("env")
	got, _ := c.doEnvBuiltin([]string{"list"}, "")
	if len(got) != 0 {
		t.Errorf("expected no candidates after 'list', got %v", got)
	}
}

// --- envBuiltinKeys ---

func TestEnvBuiltinKeys(t *testing.T) {
	pairs := []string{"AAA=1", "BBB=2", "CCC=3"}
	got := envBuiltinKeys(pairs)
	if len(got) != 3 || got[0] != "AAA" || got[1] != "BBB" || got[2] != "CCC" {
		t.Errorf("envBuiltinKeys = %v, want [AAA BBB CCC]", got)
	}
}

func TestEnvBuiltinKeys_Empty(t *testing.T) {
	got := envBuiltinKeys(nil)
	if len(got) != 0 {
		t.Errorf("envBuiltinKeys(nil) = %v, want []", got)
	}
}
