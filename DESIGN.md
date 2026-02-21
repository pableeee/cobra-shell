# cobra-shell: A Generic Interactive Shell for Cobra CLIs

**Date:** 2026-02-21
**Status:** Draft / Exploration

---

## Problem

Cobra CLIs are excellent for scripting but awkward for interactive use. Every invocation re-parses flags, re-establishes connections, and outputs results that disappear. Power users who run many commands in sequence end up typing the binary name repeatedly and lose context between calls.

A dedicated interactive shell solves this — but today each project has to build its own (as was done in `go-cs-metrics`). The machinery is always the same: readline loop, tab completion, history. Only the commands differ.

---

## Goal

Build a standalone Go library (and optional binary) that wraps **any Cobra CLI** in an interactive shell, with zero changes required to the target binary. The host binary is treated as a black box; the shell discovers its structure at runtime through Cobra's built-in completion protocol.

---

## Prior Art

| Project | What it does | Gap |
|---------|-------------|-----|
| `github.com/abiosoft/ishell` | Build interactive shells from scratch | Requires manual command registration |
| `github.com/c-bata/go-prompt` | Rich prompt with completion | Same — no auto-discovery |
| `github.com/spf13/cobra` shell completion | Generates bash/zsh/fish scripts | Not a REPL |
| Hand-rolled shells (e.g. this repo) | Full control, stateful | Per-project, not reusable |

**The gap:** nothing auto-discovers a Cobra binary's command tree and wraps it in a REPL without code changes to the target.

---

## Cobra Completion Protocol

Cobra injects a hidden `__complete` command into every binary. It accepts the current argument list and returns completions on stdout:

```
$ mybinary __complete "sub"
subcommand1
subcommand2
:4
Completion ended with directive: ShellCompDirectiveNoFileComp
```

The last line is always `:N` where N is a `ShellCompDirective` bitmask:

| Bit | Name | Meaning |
|-----|------|---------|
| 0 | `Default` | Let the shell also complete files |
| 1 | `NoSpace` | Don't append a space after the completion |
| 2 | `NoFileComp` | Suppress file completions |
| 4 | `FilterFileExt` | Only complete files with given extensions |
| 8 | `FilterDirs` | Only complete directories |
| 16 | `KeepOrder` | Preserve completion order |
| 32 | `Error` | An error occurred; suppress completions |

Parsing this is sufficient to drive a fully functional tab-completion system without any knowledge of the binary's internals.

---

## Architecture

```
┌────────────────────────────────────────────────────────────┐
│                        cobra-shell                         │
│                                                            │
│  Config                                                    │
│  ├── BinaryPath  string                                    │
│  ├── Prompt      string         (default: "> ")            │
│  ├── HistoryFile string         (default: ~/.binary_hist)  │
│  ├── Env         []string       (extra env vars)           │
│  └── Hooks       Hooks          (see below)                │
│                                                            │
│  Shell                                                     │
│  ├── readline instance (github.com/chzyer/readline)        │
│  ├── Completer  → calls binary __complete                  │
│  ├── Executor   → calls binary <args>, streams output      │
│  └── History    → persisted to file                        │
└────────────────────────────────────────────────────────────┘
```

### Completer

On every tab press, the completer:

1. Splits the current line into tokens.
2. Calls `exec.Command(binary, append([]string{"__complete"}, tokens...)...)`.
3. Parses the output: lines before `:N` are candidates; the directive controls file fallback.
4. Returns the candidates to readline.

The call is made with a short timeout (default 500 ms) to avoid stalling on slow binaries.

### Executor

On enter:

1. Tokenizes the input (respecting quotes).
2. Runs `exec.Command(binary, tokens...)` with stdin/stdout/stderr inherited from the terminal.
3. Waits for exit. Non-zero exit codes are printed but do not terminate the shell.

### Hooks

```go
type Hooks struct {
    // Called before executing a command. Return false to cancel.
    BeforeExec func(args []string) bool

    // Called after executing a command with its exit code.
    AfterExec func(args []string, exitCode int)

    // Called when the shell starts, useful for printing a banner.
    OnStart func(shell *Shell)

    // Called on exit.
    OnExit func()
}
```

Hooks allow the caller to inject stateful behavior (e.g., updating a status line, logging, injecting auth tokens) without the library knowing anything about the domain.

---

## API Design

### Minimal usage (library mode)

```go
import "github.com/you/cobra-shell"

func main() {
    sh := cobrashell.New(cobrashell.Config{
        BinaryPath: "/usr/local/bin/mybinary",
        Prompt:     "mybinary> ",
    })
    sh.Run()
}
```

### With hooks

```go
sh := cobrashell.New(cobrashell.Config{
    BinaryPath:  os.Args[0], // wrap yourself
    Prompt:      "myapp> ",
    HistoryFile: filepath.Join(os.UserHomeDir(), ".myapp_history"),
    Hooks: cobrashell.Hooks{
        AfterExec: func(args []string, code int) {
            if code != 0 {
                fmt.Printf("[exit %d]\n", code)
            }
        },
    },
})
```

### Embedded shell command

For Cobra CLIs that want to ship a `shell` subcommand:

```go
// In your root command setup:
rootCmd.AddCommand(cobrashell.Command(cobrashell.Config{
    BinaryPath: os.Args[0],
    Prompt:     "myapp> ",
}))
```

This is the pattern used in `go-cs-metrics` today, but it would be a one-liner instead of ~100 lines of custom code.

### Standalone binary

A `cobra-shell` binary that wraps any installed CLI:

```sh
cobra-shell --binary kubectl --prompt "k8s> "
cobra-shell --binary gh
```

---

## Completion Quality Tiers

Not all Cobra binaries have equally rich completions. The shell degrades gracefully:

| What the binary provides | Shell behavior |
|--------------------------|----------------|
| Full `__complete` support (Cobra ≥ 1.2) | Full dynamic completion |
| Partial (only subcommands, no flag values) | Subcommand + flag name completion; values fall back to files |
| No `__complete` (non-Cobra or old Cobra) | Falls back to `--help` parsing (best-effort) |
| Nothing | Raw readline with history only |

The `--help` fallback parses the `Commands:` and `Flags:` sections of help text using a small heuristic parser. It is good enough for subcommand navigation but cannot complete flag values.

---

## Stateful Shell (Advanced / Optional)

The generic model above treats the binary as a black box. An optional embedded mode allows the library to be used **inside** the same process, enabling:

- Shared in-memory state (DB handles, caches)
- Richer completions sourced from live data (e.g., listing DB IDs for `show <id>`)
- No subprocess overhead per command

This is done by registering a `cobra.Command` tree directly instead of a binary path:

```go
sh := cobrashell.NewEmbedded(cobrashell.EmbeddedConfig{
    RootCmd: rootCmd, // *cobra.Command
    Prompt:  "myapp> ",
    DynamicCompletions: map[string]cobrashell.CompletionFunc{
        "show": func() []string { return db.ListHashPrefixes() },
    },
})
```

This embedded mode is how `go-cs-metrics`'s current shell works; the library would just formalize it.

---

## Out of Scope (v1)

- Piping between commands (`list | grep foo`) — requires a real shell parser
- Aliasing / macro recording
- Multi-line input
- PTY allocation for interactive subcommands (e.g. `vim`, `less`)
- Non-Cobra CLIs beyond the `--help` fallback

---

## Open Questions

1. **Completion timeout**: 500 ms default — is this right for slow binaries (e.g., those that hit a network)?
2. **Token splitting**: needs to handle single/double quotes, backslash escapes. Use `github.com/google/shlex` or roll minimal parser?
3. **PTY for color output**: some binaries detect non-TTY stdout and disable color. Should we allocate a PTY for the subprocess? `github.com/creack/pty` would handle this but adds complexity.
4. **Windows support**: `__complete` works cross-platform; readline is trickier on Windows. Scope to Unix for v1?
5. **Module path**: `github.com/pable/cobra-shell`? Pick a name before wiring up examples.

---

## Milestones

| # | Milestone | Deliverable |
|---|-----------|-------------|
| 1 | Prototype | Wrap a single hardcoded binary; tab completion works |
| 2 | Library API | `cobrashell.New(Config)` + `Run()` published |
| 3 | Embedded mode | `NewEmbedded` with `*cobra.Command` tree |
| 4 | `--help` fallback | Graceful degradation for non-Cobra binaries |
| 5 | Standalone binary | `cobra-shell --binary <path>` |
| 6 | `Command()` helper | One-liner `shell` subcommand for Cobra CLIs |

---

## References

- [Cobra shell completion docs](https://cobra.dev/completions/)
- `ShellCompDirective` constants: `github.com/spf13/cobra/completions.go`
- `github.com/chzyer/readline` — readline with completion callbacks
- `github.com/google/shlex` — POSIX-ish tokenizer
- `github.com/creack/pty` — PTY allocation if needed
