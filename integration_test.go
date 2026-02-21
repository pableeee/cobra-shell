package cobrashell

// Integration tests exercise the full completion and execution pipeline
// against a real Cobra binary (testdata/testbin). TestMain compiles it once
// into a temp directory; individual tests skip when compilation fails (e.g.
// restricted CI environments without access to the Go toolchain at test time).

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// testBinary holds the path to the compiled testbin binary. Populated by
// TestMain; empty when compilation fails (tests that need it call t.Skip).
var testBinary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "cobra-shell-integration-*")
	if err == nil {
		testBinary = filepath.Join(dir, "testbin")
		if buildErr := exec.Command("go", "build", "-o", testBinary, "./testdata/testbin").Run(); buildErr != nil {
			testBinary = ""
		}
	}

	code := m.Run()

	if dir != "" {
		_ = os.RemoveAll(dir)
	}
	os.Exit(code)
}

// newIntegrationShell returns a Shell wired to testBinary. It bypasses New()
// to avoid binary resolution overhead and to allow internal field access.
func newIntegrationShell() *Shell {
	return &Shell{
		cfg:        Config{CompletionTimeout: defaultCompletionTimeout},
		binary:     testBinary,
		sessionEnv: make(map[string]string),
	}
}

// --- Completion ---

func TestIntegration_TryComplete_Subcommands(t *testing.T) {
	if testBinary == "" {
		t.Skip("testbin not compiled")
	}
	c := &completer{shell: newIntegrationShell()}

	candidates, _, ok := c.tryComplete(nil, "gr")
	if !ok {
		t.Fatal("tryComplete returned ok=false; binary may not support __completeNoDesc")
	}
	found := false
	for _, cand := range candidates {
		if cand == "greet" {
			found = true
		}
	}
	if !found {
		t.Errorf("tryComplete(nil, 'gr') = %v, want 'greet' among candidates", candidates)
	}
}

func TestIntegration_TryComplete_AllSubcommands(t *testing.T) {
	if testBinary == "" {
		t.Skip("testbin not compiled")
	}
	c := &completer{shell: newIntegrationShell()}

	candidates, _, ok := c.tryComplete(nil, "")
	if !ok {
		t.Fatal("tryComplete returned ok=false")
	}
	names := make(map[string]bool, len(candidates))
	for _, cand := range candidates {
		names[cand] = true
	}
	for _, want := range []string{"greet", "fail", "echo"} {
		if !names[want] {
			t.Errorf("expected %q in candidates %v", want, candidates)
		}
	}
	if names["hidden"] {
		t.Errorf("hidden command should not appear in candidates %v", candidates)
	}
}

func TestIntegration_TryComplete_Flags(t *testing.T) {
	if testBinary == "" {
		t.Skip("testbin not compiled")
	}
	c := &completer{shell: newIntegrationShell()}

	candidates, _, ok := c.tryComplete([]string{"greet"}, "--na")
	if !ok {
		t.Fatal("tryComplete returned ok=false")
	}
	found := false
	for _, cand := range candidates {
		if cand == "--name" {
			found = true
		}
	}
	if !found {
		t.Errorf("tryComplete(['greet'], '--na') = %v, want '--name' among candidates", candidates)
	}
}

func TestIntegration_CompleterDo_ReturnsSuffix(t *testing.T) {
	if testBinary == "" {
		t.Skip("testbin not compiled")
	}
	// Regression test: Do() must return suffixes so readline does not double
	// the already-typed prefix. Typing "gr" + Tab should complete to "greet",
	// not "grgreet".
	c := &completer{shell: newIntegrationShell()}
	line := []rune("gr")
	candidates, length := c.Do(line, len(line))

	if length != 2 {
		t.Errorf("length = %d, want 2 (len of 'gr')", length)
	}
	for _, cand := range candidates {
		if string(cand) == "greet" {
			t.Errorf("Do returned full word %q; want suffix %q", "greet", "eet")
		}
	}
	found := false
	for _, cand := range candidates {
		if string(cand) == "eet" {
			found = true
		}
	}
	if !found {
		t.Errorf("suffix 'eet' not found in candidates %v", candidates)
	}
}

func TestIntegration_TryComplete_NoMatch(t *testing.T) {
	if testBinary == "" {
		t.Skip("testbin not compiled")
	}
	c := &completer{shell: newIntegrationShell()}

	candidates, _, ok := c.tryComplete(nil, "zzz")
	if !ok {
		t.Fatal("tryComplete returned ok=false")
	}
	if len(candidates) != 0 {
		t.Errorf("tryComplete(nil, 'zzz') = %v, want empty", candidates)
	}
}

// --- Execution ---

func TestIntegration_Execute_Success(t *testing.T) {
	if testBinary == "" {
		t.Skip("testbin not compiled")
	}
	var gotCode int
	sh := &Shell{
		cfg: Config{
			Hooks: Hooks{
				AfterExec: func(_ []string, code int) { gotCode = code },
			},
		},
		binary:     testBinary,
		sessionEnv: make(map[string]string),
	}
	sh.execute("greet")
	if gotCode != 0 {
		t.Errorf("execute('greet') exit code = %d, want 0", gotCode)
	}
}

func TestIntegration_Execute_NonZeroExitCode(t *testing.T) {
	if testBinary == "" {
		t.Skip("testbin not compiled")
	}
	var gotCode int
	sh := &Shell{
		cfg: Config{
			Hooks: Hooks{
				AfterExec: func(_ []string, code int) { gotCode = code },
			},
		},
		binary:     testBinary,
		sessionEnv: make(map[string]string),
	}
	sh.execute("fail")
	if gotCode == 0 {
		t.Error("execute('fail') exit code = 0, want non-zero")
	}
}

func TestIntegration_Execute_BeforeExecCancels(t *testing.T) {
	if testBinary == "" {
		t.Skip("testbin not compiled")
	}
	afterCalled := false
	sh := &Shell{
		cfg: Config{
			Hooks: Hooks{
				BeforeExec: func(_ []string) error {
					return fmt.Errorf("blocked")
				},
				AfterExec: func(_ []string, _ int) { afterCalled = true },
			},
		},
		binary:     testBinary,
		sessionEnv: make(map[string]string),
	}
	sh.execute("greet")
	if afterCalled {
		t.Error("AfterExec should not be called when BeforeExec cancels")
	}
}

func TestIntegration_Execute_EnvBuiltinNotForwarded(t *testing.T) {
	if testBinary == "" {
		t.Skip("testbin not compiled")
	}
	// The env built-in should be consumed without invoking the binary.
	// We verify AfterExec is NOT called (env built-in bypasses it).
	afterCalled := false
	sh := &Shell{
		cfg: Config{
			EnvBuiltin: "env",
			Hooks: Hooks{
				AfterExec: func(_ []string, _ int) { afterCalled = true },
			},
		},
		binary:     testBinary,
		sessionEnv: make(map[string]string),
	}
	sh.execute("env list")
	if afterCalled {
		t.Error("AfterExec should not be called for env built-in commands")
	}
}
