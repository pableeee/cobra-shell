# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`cobra-shell` (module `github.com/pable/cobra-shell`) is a Go library that wraps any Cobra CLI binary in an interactive REPL shell. The key insight: every Cobra binary automatically has a hidden `__complete` command that returns tab completions; this library uses that to drive a fully functional interactive shell without requiring changes to the target binary.

**Current status:** Design phase — `go.mod` and `DESIGN.md` exist, no source files yet. See `DESIGN.md` for the full spec.

## Commands

```sh
make build          # build standalone binary → bin/cobra-shell
make test           # run all tests
make test-verbose   # run all tests with -v
make test-run RUN=TestParseHelp  # run a single test by name
make vet            # go vet
make lint           # golangci-lint (must be installed separately)
make install        # go install the standalone binary
make clean          # remove bin/
```

## Architecture

Two operating modes are planned:

### 1. Subprocess mode (primary)
Wraps an external binary as a black box. On each tab press, the `Completer` calls `binary __completeNoDesc <tokens>` and parses the output. On enter, the `Executor` runs `binary <tokens>` with inherited stdin/stdout/stderr. Empty input is a no-op. Ctrl-C sends SIGINT to the child only; Ctrl-D exits the shell.

### 2. Embedded mode (`NewEmbedded`)
Operates in-process by accepting a `*cobra.Command` tree directly. Allows shared memory state (DB handles, caches) and dynamic completions sourced from live data. Completion walks the tree via `cobra.Command.Traverse`; calls `ValidArgsFunction` and `DynamicCompletions`. Flags are reset to defaults via `resetCommandTree` before each `Execute()` call.

### Core types

- **`Config`** — `BinaryPath` (resolved to abs path at `New()`), `Prompt`, `HistoryFile` (default `~/.{basename}_history`), `Env` (additive to inherited env), `EnvBuiltin` (opt-in env built-in name, default `""`), `Hooks`
- **`Shell`** — holds the readline instance, Completer, Executor, History, and `sessionEnv map[string]string`
- **`Hooks`** — `BeforeExec func([]string) error` (non-nil cancels + prints reason), `AfterExec`, `OnStart`, `OnExit`
- **`EmbeddedConfig`** — `RootCmd *cobra.Command`, `Prompt`, `DynamicCompletions map[string]CompletionFunc`, `Hooks EmbeddedHooks`
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

## Planned Dependencies

- `github.com/chzyer/readline` — readline with completion callbacks
- `github.com/google/shlex` — POSIX-ish token splitting (or a minimal custom parser)
- `github.com/creack/pty` — PTY allocation for color output (optional, adds complexity)
- `github.com/spf13/cobra` — for embedded mode

## Public API Surface (from design)

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
- **PTY:** No PTY for v1. Side effects (pager launches, signal complexity, Windows incompatibility) outweigh the color benefit. Workaround: set `FORCE_COLOR=1` or equivalent via `Config.Env`.
- **Windows:** Unix-only for v1. `chzyer/readline` and Unix signal semantics are not portable enough to include in scope.
- **Module path:** `github.com/pable/cobra-shell` (set in `go.mod`).
- **Out of scope for v1:** pipes between commands, aliasing, multi-line input, PTY for interactive subcommands (e.g. `vim`).
