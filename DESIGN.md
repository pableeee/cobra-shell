# cobra-shell: A Generic Interactive Shell for Cobra CLIs

**Date:** 2026-02-21
**Status:** Final

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

Cobra injects two hidden commands into every binary: `__complete` (with descriptions) and `__completeNoDesc` (without). They accept the full token list the user has typed so far, with the **last token being the partial word being completed**. Each token is a separate argument — not a single quoted string.

```
# User typed: mybinary sub[TAB]
$ mybinary __completeNoDesc sub
subcommand1
subcommand2
:4

# User typed: mybinary sub [TAB]  (trailing space → empty partial)
$ mybinary __completeNoDesc sub ""
subcommand1
subcommand2
:4
```

`__complete` returns the same but with optional tab-separated descriptions on each completion line:

```
subcommand1	does the first thing
subcommand2	does the second thing
:4
```

**We use `__completeNoDesc`** because readline has no UI for displaying per-completion descriptions, so stripping them is both simpler and avoids parsing the tab separator.

Cobra performs **server-side prefix filtering**: it only returns completions that match the partial word, so the client never needs to filter the result set.

The final line is always `:N` where N is a `ShellCompDirective` bitmask. The values (defined as `1 << iota` starting from 1) are:

| Value | Name | Meaning |
|-------|------|---------|
| 0 | `Default` | Allow file completion as fallback |
| 1 | `Error` | Completion failed; suppress results |
| 2 | `NoSpace` | Don't append a space after the completion |
| 4 | `NoFileComp` | Suppress file completion fallback |
| 8 | `FilterFileExt` | Only complete files with given extensions |
| 16 | `FilterDirs` | Only complete directories |
| 32 | `KeepOrder` | Preserve completion order |

Parsing this output is sufficient to drive a fully functional tab-completion system without any knowledge of the binary's internals.

---

## Architecture

```
┌────────────────────────────────────────────────────────────┐
│                        cobra-shell                         │
│                                                            │
│  Config                                                    │
│  ├── BinaryPath         string        (abs path at New())  │
│  ├── Prompt             string        (default: "> ")      │
│  ├── HistoryFile        string        (default: see below) │
│  ├── Env                []string      (additive to env)    │
│  ├── CompletionTimeout  time.Duration (default: 500ms)     │
│  └── Hooks              Hooks         (see below)          │
│                                                            │
│  Shell                                                     │
│  ├── readline instance (github.com/chzyer/readline)        │
│  ├── Completer  → calls binary __completeNoDesc            │
│  ├── Executor   → calls binary <args>, streams output      │
│  └── History    → persisted to file                        │
└────────────────────────────────────────────────────────────┘
```

### Completer

`chzyer/readline` calls the completer with `(line []rune, pos int)` — the full line and cursor position. On every tab press, the completer:

1. Truncates the line to the cursor position (`line[:pos]`).
2. Tokenizes the truncated line using `github.com/google/shlex` (handles single/double quotes and backslash escapes). The last token is `toComplete` (the partial word); all preceding tokens are the context args. If the line ends with whitespace, `toComplete` is an empty string.
3. Calls `exec.Command(binary, append([]string{"__completeNoDesc"}, append(contextArgs, toComplete)...)...)` under a `context.WithTimeout` of `Config.CompletionTimeout` (default 500 ms).
4. Reads stdout, discards the trailing `:N` directive line, and collects the remaining lines as candidates.
5. Checks the directive: if `Error` bit is set, returns no candidates. If `Default` bit is set (file fallback allowed), may augment with filesystem completions.
6. Returns the candidates to readline, along with the length of `toComplete` so readline knows how much of the line to replace.

### Executor

On enter:

1. If the input is empty or whitespace-only, it is a no-op (no subprocess, no history entry).
2. Tokenizes the input with `github.com/google/shlex` (same library as the completer, consistent quoting semantics).
3. Runs `exec.Command(binary, tokens...)` with stdin/stdout/stderr inherited from the terminal.
4. Waits for exit. Non-zero exit codes are printed but do not terminate the shell.

**Signal handling:** Ctrl-C while a command is running sends SIGINT to the child process only; it does not exit the shell. Ctrl-C at an empty prompt clears the line (readline default). Ctrl-D at an empty prompt exits the shell.

### Hooks

```go
type Hooks struct {
    // Called before executing a command.
    // Return a non-nil error to cancel execution; the error message is printed to the user.
    // Return nil to proceed.
    BeforeExec func(args []string) error

    // Called after executing a command with its exit code.
    AfterExec func(args []string, exitCode int)

    // Called when the shell starts, useful for printing a banner.
    OnStart func(shell *Shell)

    // Called on exit.
    OnExit func()
}
```

Hooks allow the caller to inject stateful behavior (e.g., updating a status line, logging, injecting auth tokens) without the library knowing anything about the domain. Using `error` for `BeforeExec` (rather than `bool`) lets the hook surface a reason to the user when cancelling — e.g., `"auth token expired, run login first"`.

---

## API Design

### Minimal usage (library mode)

```go
import "github.com/pable/cobra-shell"

func main() {
    sh := cobrashell.New(cobrashell.Config{
        BinaryPath: "/usr/local/bin/mybinary",
        Prompt:     "mybinary> ",
    })
    if err := sh.Run(); err != nil {
        log.Fatal(err)
    }
}
```

`Run()` returns an `error` if initialisation fails (binary not found, readline setup failure, history file unwritable). It returns `nil` on a clean exit.

### With hooks

```go
sh := cobrashell.New(cobrashell.Config{
    BinaryPath:  os.Args[0], // wrap yourself
    Prompt:      "myapp> ",
    HistoryFile: filepath.Join(os.UserHomeDir(), ".myapp_history"),
    Hooks: cobrashell.Hooks{
        BeforeExec: func(args []string) error {
            if !auth.TokenValid() {
                return fmt.Errorf("auth token expired — run 'login' first")
            }
            return nil
        },
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

`New()` resolves `BinaryPath` to an absolute path immediately (via `exec.LookPath` if it has no path separator, otherwise `filepath.Abs`). This makes `os.Args[0]` safe even if the CWD changes during the shell session.

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
        // Signature matches Cobra's ValidArgsFunction for consistency.
        "show": func(args []string, toComplete string) []string {
            return db.ListHashPrefixes()
        },
    },
})
```

`CompletionFunc` is `func(args []string, toComplete string) []string`, mirroring Cobra's own `ValidArgsFunction` signature. `args` contains the tokens already accepted; `toComplete` is the partial word. This gives dynamic completions enough context to filter or generate values meaningfully.

This embedded mode is how `go-cs-metrics`'s current shell works; the library would just formalize it.

---

## Out of Scope (v1)

- Piping between commands (`list | grep foo`) — requires a real shell parser
- Aliasing / macro recording
- Multi-line input
- PTY allocation — covers both color output (some binaries disable color when stdout is not a TTY) and interactive subcommands (e.g. `vim`, `less`). Workaround for color: set `FORCE_COLOR=1` or equivalent via `Config.Env`. PTY via `github.com/creack/pty` is deferred because it triggers pager launches, complicates signal forwarding, and conflicts with the Unix-only scope.
- Windows support — `__complete` is cross-platform but `chzyer/readline` and Unix signal semantics are not. Scoped to Unix for v1.
- Non-Cobra CLIs beyond the `--help` fallback

---

## Milestones

| # | Milestone | Deliverable |
|---|-----------|-------------|
| 1 | Prototype | Wrap a single hardcoded binary; tab completion works |
| 2 | Library API | `cobrashell.New(Config)` + `Run()` published |
| 3 | `--help` fallback | Graceful degradation for non-Cobra binaries |
| 4 | Standalone binary | `cobra-shell --binary <path>` |
| 5 | `Command()` helper | One-liner `shell` subcommand for Cobra CLIs |
| 6 | Embedded mode | `NewEmbedded` with `*cobra.Command` tree |

---

## References

- [Cobra shell completion docs](https://cobra.dev/completions/)
- `ShellCompDirective` constants: `github.com/spf13/cobra/completions.go`
- `github.com/chzyer/readline` — readline with completion callbacks
- `github.com/google/shlex` — POSIX-ish tokenizer
- `github.com/creack/pty` — PTY allocation (post-v1)
