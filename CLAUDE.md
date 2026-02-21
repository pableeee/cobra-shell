# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`cobra-shell` (module `github.com/pable/cobra-shell`) is a Go library that wraps any Cobra CLI binary in an interactive REPL shell. The key insight: every Cobra binary automatically has a hidden `__complete` command that returns tab completions; this library uses that to drive a fully functional interactive shell without requiring changes to the target binary.

**Current status:** Fully implemented. See `DESIGN.md` for the rationale and `adr/` for architectural decision records.

## Commands

```sh
make build               # build standalone binary → bin/cobra-shell
make test                # run all tests (unit + integration)
make test-verbose        # run all tests with -v
make test-run RUN=TestParseHelp  # run a single test by name
make test-race           # run all tests with race detector
make vet                 # go vet
make lint                # golangci-lint (must be installed separately)
make install             # go install the standalone binary
make clean               # remove bin/
```

CI runs `go vet ./...` + `go test -race -count=1 ./...` + `go build ./...` on every push/PR via `.github/workflows/ci.yml`.

## Architecture

Two operating modes are planned:

### 1. Subprocess mode (primary)
Wraps an external binary as a black box. On each tab press, the `Completer` calls `binary __completeNoDesc <tokens>` and parses the output (plain subprocess, no PTY). On enter, `spawnCommand` allocates a PTY when stdin is a real terminal (`term.IsTerminal`): the parent goes into raw mode and copies bytes bidirectionally; the PTY slave's line discipline delivers Ctrl-C as SIGINT to the subprocess. Falls back to plain exec (with parent SIGINT suppression) when stdin is not a terminal or PTY creation fails. Empty input is a no-op. Ctrl-D exits the shell.

### 2. Embedded mode (`NewEmbedded`)
Operates in-process by accepting a `*cobra.Command` tree directly. Allows shared memory state (DB handles, caches) and dynamic completions sourced from live data. Completion walks the tree via `cobra.Command.Traverse`; calls `ValidArgsFunction` and `DynamicCompletions`. Flags are reset to defaults via `resetCommandTree` before each `Execute()` call.

### Core types

- **`Config`** — `BinaryPath` (resolved to abs path at `New()`), `Prompt`, `HistoryFile` (default `~/.{basename}_history`), `Env` (additive to inherited env), `EnvBuiltin` (opt-in env built-in name, default `""`), `DynamicPrompt func(lastExitCode int) string` (when set, overrides `Prompt`; called after each command; use `Colorize()` for ANSI colors), `Hooks`
- **`Shell`** — holds the readline instance, Completer, Executor, History, `sessionEnv map[string]string`, and `lastExitCode int`
- **`Hooks`** — `BeforeExec func([]string) error` (non-nil cancels + prints reason), `AfterExec`, `OnStart`, `OnExit`
- **`EmbeddedConfig`** — `RootCmd *cobra.Command`, `Prompt`, `DynamicPrompt func(lastExitCode int) string`, `DynamicCompletions map[string]CompletionFunc`, `Hooks EmbeddedHooks`
- **`EmbeddedHooks`** — same shape as `Hooks` but `OnStart func(*EmbeddedShell)`; separate type because the two shell types are not interchangeable
- **`CompletionFunc`** — `func(args []string, toComplete string) []string`, mirrors Cobra's `ValidArgsFunction`
- **`resetCommandTree`** — walks the command tree resetting all `pflag.Flag` values to `DefValue` and clearing `Changed`; called before each embedded `Execute()`

### Session environment variables (subprocess mode only)

`Shell.SetEnv(key, value)` / `Shell.UnsetEnv(key)` / `Shell.SessionEnv() []string` manage a `map[string]string` on `Shell`. `buildEnv()` merges three layers at spawn time: `os.Environ()` < `Config.Env` < `sessionEnv` (last value wins in `Cmd.Env`). `os.Setenv` is never called.

When `Config.EnvBuiltin` is non-empty (e.g. `"env"`), the named command is intercepted in `execute()` **before** `BeforeExec` and before spawning the binary. Subcommands: `list`, `set KEY VALUE`, `unset KEY`. The completer intercepts the same token in `Do()` to provide subcommand and key candidates. Embedded mode does not expose session env.

### Cobra `__complete` / `__completeNoDesc` protocol

Each token the user has typed is passed as a **separate argument**; the **last argument is the partial word** being completed (empty string if the line ends with whitespace). We use `__completeNoDesc` because readline cannot display per-completion descriptions.

Cobra does server-side prefix filtering — only matching completions are returned; no client-side filtering needed.

Output: one completion per line, then a directive line `:N`. `ShellCompDirective` bitmask values (1 << iota from 1): 0=Default, 1=Error, 2=NoSpace, 4=NoFileComp, 8=FilterFileExt, 16=FilterDirs, 32=KeepOrder.

### Completion quality tiers

The library degrades gracefully: full `__complete` → partial (subcommands/flags only) → `--help` heuristic parsing → readline with history only.

## Dependencies

- `github.com/chzyer/readline` — readline with completion callbacks
- `github.com/google/shlex` — POSIX-ish token splitting
- `github.com/creack/pty` — PTY allocation (subprocess mode)
- `golang.org/x/term` — `IsTerminal` + `MakeRaw` for PTY and colored stderr detection
- `github.com/spf13/cobra` + `pflag` — embedded mode

### Colors and prompt

`prompt.go` exports `Colorize(text, code string) string` and color constants (`ColorRed`, `ColorGreen`, `ColorYellow`, `ColorBlue`, `ColorMagenta`, `ColorCyan`, `ColorBold`, `ColorReset`). `Colorize` wraps the text in readline's `\x01`/`\x02` ignore markers so cursor positioning stays correct. Internal `writeErr()` prints cobra-shell's own error messages in red when stderr is a terminal.

## Public API Surface

```go
// Subprocess mode — Run() returns error on init failure, nil on clean exit
if err := cobrashell.New(Config{BinaryPath: ..., Prompt: ...}).Run(); err != nil {
    log.Fatal(err)
}

// Embedded mode
cobrashell.NewEmbedded(EmbeddedConfig{RootCmd: rootCmd, ...}).Run()

// One-liner shell subcommand for Cobra CLIs
// New() resolves os.Args[0] to an absolute path immediately
rootCmd.AddCommand(cobrashell.Command(Config{BinaryPath: os.Args[0]}))
```

## Design Decisions

- **Completion timeout:** `Config.CompletionTimeout time.Duration`, default 500 ms. Implemented via `context.WithTimeout` on the `__completeNoDesc` subprocess call.
- **Token splitting:** `github.com/google/shlex` used in both the Completer and Executor for consistent POSIX quoting semantics.
- **PTY:** Implemented (see ADR-007). Auto-detected via `term.IsTerminal`; plain fallback for non-TTY stdin. PTY path puts the parent terminal in raw mode so Ctrl-C flows as byte 0x03 through the PTY slave's line discipline → SIGINT for the subprocess; no `signal.Notify` needed in the parent for the PTY path.
- **Windows:** Unix-only for v1. `chzyer/readline` and Unix signal semantics are not portable enough to include in scope.
- **Module path:** `github.com/pable/cobra-shell` (set in `go.mod`).
- **Out of scope for v1:** pipes between commands, aliasing, multi-line input, PTY for interactive subcommands (e.g. `vim`).
