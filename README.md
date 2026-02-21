# cobra-shell

An interactive shell for any [Cobra](https://cobra.dev) CLI — with tab completion and persistent history — requiring **zero changes to the target binary**.

```
$ cobra-shell --binary kubectl --prompt "k8s> "
k8s> get po[TAB]
pods  poddisruptionbudgets  podtemplates
k8s> get pods -n kube-system
NAME                               READY   STATUS    RESTARTS   AGE
coredns-5d78c9869d-p9f2k           1/1     Running   0          3d
...
k8s> ▌
```

## How it works

Every Cobra binary (≥ v1.2) automatically exposes a hidden `__completeNoDesc` command. cobra-shell calls it on every Tab press to get context-aware completions — subcommands, flags, and dynamic values — without knowing anything about the binary's internals. Command execution spawns the binary as a subprocess with stdin/stdout/stderr inherited from the terminal.

## Installation

### Library

```sh
go get github.com/pable/cobra-shell
```

### Standalone binary

```sh
go install github.com/pable/cobra-shell/cmd/cobra-shell@latest
```

## Usage

### Library mode

```go
import cobrashell "github.com/pable/cobra-shell"

func main() {
    sh := cobrashell.New(cobrashell.Config{
        BinaryPath: "/usr/local/bin/kubectl",
        Prompt:     "k8s> ",
    })
    if err := sh.Run(); err != nil {
        log.Fatal(err)
    }
}
```

### Shell subcommand

Add a `shell` subcommand to your own Cobra CLI with one line:

```go
rootCmd.AddCommand(cobrashell.Command(cobrashell.Config{
    BinaryPath: os.Args[0], // wrap yourself
    Prompt:     "myapp> ",
}))
```

Users then run `myapp shell` to enter an interactive session.

### Hooks

```go
sh := cobrashell.New(cobrashell.Config{
    BinaryPath:  os.Args[0],
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
        OnStart: func(sh *cobrashell.Shell) {
            fmt.Println("Welcome! Type 'exit' or Ctrl-D to quit.")
        },
    },
})
```

### Standalone binary

```sh
cobra-shell --binary kubectl --prompt "k8s> "
cobra-shell --binary gh
cobra-shell --binary ./myapp --timeout 2s
```

## Session environment variables

Many CLI tools use environment variables for credentials or configuration.
cobra-shell lets you manage these at the prompt without restarting the shell.

Enable the built-in by setting `Config.EnvBuiltin`:

```go
sh := cobrashell.New(cobrashell.Config{
    BinaryPath: "/usr/local/bin/heroku",
    Prompt:     "heroku> ",
    EnvBuiltin: "env",
})
```

Inside the shell:

```
heroku> env set HEROKU_API_TOKEN secret123
heroku> env list
HEROKU_API_TOKEN=secret123
heroku> heroku apps          # token is injected into the subprocess
heroku> env unset HEROKU_API_TOKEN
```

Session variables are merged into the subprocess environment at spawn time,
with precedence over `os.Environ()` and `Config.Env`. `os.Setenv` is never
called — the current process is unaffected.

Tab-completion is available for `env list`, `env set`, and `env unset KEY`
(keys from the current session are offered).

You can also manage session env programmatically:

```go
sh.SetEnv("MY_TOKEN", token)
sh.UnsetEnv("MY_TOKEN")
pairs := sh.SessionEnv() // sorted ["KEY=VALUE", ...]
```

> **Note:** Session env is a subprocess-mode feature. Embedded mode does not
> expose it because in-process commands share the same OS environment.

## Configuration

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `BinaryPath` | `string` | *(required)* | Path or bare name of the binary to wrap. Resolved to an absolute path by `New`. |
| `Prompt` | `string` | `"> "` | Prompt string displayed before each input line. |
| `HistoryFile` | `string` | `~/.<binary>_history` | File for persistent command history. |
| `Env` | `[]string` | `nil` | Extra environment variables (`"KEY=VALUE"`), additive to the current environment. |
| `CompletionTimeout` | `time.Duration` | `500ms` | Maximum time to wait for `__completeNoDesc`. Increase for network-backed binaries. |
| `EnvBuiltin` | `string` | `""` | When non-empty, enables the built-in env management command with this name. |
| `Hooks` | `Hooks` | — | Lifecycle callbacks; all fields optional. |

## Keyboard shortcuts

| Key | Action |
|-----|--------|
| Tab | Complete current word |
| Ctrl-C | Cancel current command (child receives SIGINT); shell continues |
| Ctrl-D | Exit the shell |
| ↑ / ↓ | Navigate history |
| Ctrl-R | Reverse history search |

## Completion quality

Not all Cobra binaries register dynamic completions. The shell degrades gracefully:

| Binary capability | Shell behaviour |
|-------------------|-----------------|
| Full `__completeNoDesc` (Cobra ≥ 1.2) | Full dynamic completion |
| Partial (subcommands and flag names only) | Subcommand + flag name completion |
| No `__completeNoDesc` (old or non-Cobra) | History only |

## Color output

Some binaries disable color when stdout is not detected as a TTY. Set `FORCE_COLOR=1` (or the binary-specific equivalent) via `Config.Env`:

```go
Env: []string{"FORCE_COLOR=1"},
```

PTY allocation is deferred to a future release (see [ADR-003](adr/003-no-pty-v1.md)).

## Known limitations (v1)

- **Unix only.** `chzyer/readline` and Unix signal semantics are not portable to Windows.
- **No pipes.** `list | grep foo` is not supported; the input is passed verbatim to the binary.
- **No PTY.** Interactive subcommands (`vim`, `less`, `ssh`) do not work correctly.
- **No aliasing or multi-line input.**

## Embedded mode

For CLIs that want to share in-process state (DB handles, caches) across
commands, use `NewEmbedded` instead of `New`. Commands run in the same process
via `cobra.Command.Execute`; completion walks the command tree directly rather
than spawning a subprocess.

```go
sh := cobrashell.NewEmbedded(cobrashell.EmbeddedConfig{
    RootCmd: rootCmd,
    Prompt:  "myapp> ",
    DynamicCompletions: map[string]cobrashell.CompletionFunc{
        // Live completions sourced from in-process state.
        "show": func(args []string, toComplete string) []string {
            return db.ListIDs(toComplete)
        },
    },
    Hooks: cobrashell.EmbeddedHooks{
        OnStart: func(sh *cobrashell.EmbeddedShell) {
            fmt.Println("Connected. Type 'exit' to quit.")
        },
    },
})
if err := sh.Run(); err != nil {
    log.Fatal(err)
}
```

Completion sources (in order): static subcommand names → `DynamicCompletions`
→ `ValidArgsFunction` on the matched command → flag names.

Flag state is reset to defaults between commands so that flags from one run do
not bleed into the next.

## Architecture

See [DESIGN.md](DESIGN.md) for the full design rationale and [adr/](adr/) for architectural decision records.

```
Tab press → Completer.Do()
              → shlex.Split(line[:cursor])
              → binary __completeNoDesc contextArgs... toComplete
              → parseCompletions() → candidates → readline

Enter → Shell.execute()
          → shlex.Split(line)
          → BeforeExec hook
          → exec.Command(binary, tokens...)  [SIGINT suppressed in parent]
          → AfterExec hook
```
