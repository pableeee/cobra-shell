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

### Standalone binary

Wrap any Cobra binary by name or path:

```sh
cobra-shell --binary kubectl --prompt "k8s> "
cobra-shell --binary gh --prompt "gh> "
cobra-shell --binary ./myapp
```

All flags:

```sh
cobra-shell --binary kubectl \
            --prompt "k8s> " \
            --history ~/.kubectl_shell_history \
            --timeout 2s
```

Session transcript:

```
$ cobra-shell --binary gh --prompt "gh> "
gh> pr li[TAB]
list
gh> pr list --repo cli/cli
  #1234  Fix tab completion  feature  about 2 hours ago
gh> repo clone[TAB]
clone
gh> repo clone cli/cli
Cloning into 'cli'...
gh> exit
$
```

### Library mode

Use `New` to wrap any binary and call `Run` to start the shell loop. `Run`
blocks until the user exits and returns `nil` on a clean exit.

```go
import (
    "log"
    cobrashell "github.com/pable/cobra-shell"
)

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

Session transcript:

```
k8s> get [TAB]
configmaps  deployments  ingresses  namespaces  nodes  pods  services ...
k8s> get pods --namespace [TAB]
default  kube-system  monitoring
k8s> get pods --namespace kube-system
NAME                               READY   STATUS    RESTARTS   AGE
coredns-5d78c9869d-p9f2k           1/1     Running   0          3d
k8s> exit
```

### Shell subcommand

Add a `shell` subcommand to your own Cobra CLI so users can enter an
interactive session with `myapp shell`:

```go
import (
    "os"
    cobrashell "github.com/pable/cobra-shell"
)

func init() {
    rootCmd.AddCommand(cobrashell.Command(cobrashell.Config{
        BinaryPath: os.Args[0], // wrap the binary itself
        Prompt:     "myapp> ",
    }))
}
```

Session transcript:

```
$ myapp shell
myapp> serve --po[TAB]
--port
myapp> serve --port 9090
Listening on :9090
^C
myapp> version
v1.4.2
myapp> exit
$
```

### Hooks

All four hooks are optional; nil values are silently skipped.

```go
import (
    "fmt"
    "log"
    "os"
    "path/filepath"
    cobrashell "github.com/pable/cobra-shell"
)

func main() {
    sh := cobrashell.New(cobrashell.Config{
        BinaryPath:  "/usr/local/bin/myapp",
        Prompt:      "myapp> ",
        HistoryFile: filepath.Join(os.Getenv("HOME"), ".myapp_history"),
        Hooks: cobrashell.Hooks{
            // OnStart runs once before the first prompt.
            OnStart: func(sh *cobrashell.Shell) {
                fmt.Println("Welcome! Type 'exit' or Ctrl-D to quit.")
            },
            // BeforeExec runs before each command. Return a non-nil error
            // to cancel execution and print the message to stderr.
            BeforeExec: func(args []string) error {
                if !auth.TokenValid() {
                    return fmt.Errorf("auth token expired — run 'login' first")
                }
                return nil
            },
            // AfterExec runs after each command with its exit code.
            AfterExec: func(args []string, code int) {
                if code != 0 {
                    fmt.Fprintf(os.Stderr, "[exit %d]\n", code)
                }
            },
            // OnExit runs once on a clean exit (Ctrl-D or "exit").
            OnExit: func() {
                fmt.Println("Goodbye!")
            },
        },
    })
    if err := sh.Run(); err != nil {
        log.Fatal(err)
    }
}
```

Session transcript:

```
Welcome! Type 'exit' or Ctrl-D to quit.
myapp> deploy production
Deploying to production...done.
myapp> deploy staging
Error: permission denied
[exit 1]
myapp> refresh-token   # token expired mid-session
auth token expired — run 'login' first
myapp> login
Logged in.
myapp> exit
Goodbye!
```

### Embedded mode

Use `NewEmbedded` when commands need to share in-process state (database
handles, caches, auth tokens) across invocations. Commands run via
`cobra.Command.Execute` in the same process; completion walks the command tree
directly rather than spawning a subprocess.

```go
import (
    "fmt"
    "log"
    cobrashell "github.com/pable/cobra-shell"
)

func main() {
    // rootCmd is your existing *cobra.Command tree.
    sh := cobrashell.NewEmbedded(cobrashell.EmbeddedConfig{
        RootCmd: rootCmd,
        Prompt:  "myapp> ",
        // DynamicCompletions adds live candidates sourced from in-process
        // state. Keyed by the command name returned by cmd.Name().
        DynamicCompletions: map[string]cobrashell.CompletionFunc{
            "show": func(args []string, toComplete string) []string {
                // db is accessible because we're in the same process.
                return db.ListIDs(toComplete)
            },
            "delete": func(args []string, toComplete string) []string {
                return db.ListIDs(toComplete)
            },
        },
        Hooks: cobrashell.EmbeddedHooks{
            OnStart: func(sh *cobrashell.EmbeddedShell) {
                fmt.Printf("Connected to %s. Type 'exit' to quit.\n", db.Name())
            },
            AfterExec: func(args []string, code int) {
                if code != 0 {
                    fmt.Printf("[exit %d]\n", code)
                }
            },
            OnExit: func() {
                db.Close()
            },
        },
    })
    if err := sh.Run(); err != nil {
        log.Fatal(err)
    }
}
```

Session transcript:

```
Connected to mydb. Type 'exit' to quit.
myapp> show [TAB]
user:42  user:99  order:7
myapp> show user:42
id:    42
name:  Alice
email: alice@example.com
myapp> delete [TAB]
user:42  user:99  order:7
myapp> delete user:99
Deleted.
myapp> serve --po[TAB]
--port
myapp> serve --port 9090
Listening on :9090
^C
myapp> exit
```

Completion sources (in priority order): static subcommand names →
`DynamicCompletions` → `ValidArgsFunction` on the matched command → flag
names.

Flag state is reset to defaults between commands so that flags from one run do
not bleed into the next.

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

Session transcript:

```
heroku> env set HEROKU_API_TOKEN secret123
heroku> env set HEROKU_APP myapp-staging
heroku> env list
HEROKU_API_TOKEN=secret123
HEROKU_APP=myapp-staging
heroku> env uns[TAB]
unset
heroku> env unset HEROKU_[TAB]
HEROKU_API_TOKEN  HEROKU_APP
heroku> env unset HEROKU_APP
heroku> heroku apps   # HEROKU_API_TOKEN injected into subprocess env
=== My Apps
myapp-staging
myapp-production
```

Session variables are merged into the subprocess environment at spawn time,
with precedence over `os.Environ()` and `Config.Env`. `os.Setenv` is never
called — the current process is unaffected.

You can also manage session env programmatically (e.g. from an `OnStart` hook):

```go
Hooks: cobrashell.Hooks{
    OnStart: func(sh *cobrashell.Shell) {
        sh.SetEnv("KUBECONFIG", "/etc/k8s/admin.conf")
        sh.SetEnv("NAMESPACE", "production")
    },
},
```

And inspect or clear it at runtime:

```go
sh.SetEnv("KEY", "value")    // add or overwrite
sh.UnsetEnv("KEY")           // remove
pairs := sh.SessionEnv()     // sorted ["KEY=VALUE", ...] snapshot
```

> **Note:** Session env is a subprocess-mode feature. Embedded mode does not
> expose it because in-process commands share the same OS environment.

## Configuration

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `BinaryPath` | `string` | *(required)* | Path or bare name of the binary to wrap. Resolved to an absolute path by `New`. |
| `Prompt` | `string` | `"> "` | Prompt string displayed before each input line. |
| `HistoryFile` | `string` | `~/.<binary>_history` | File for persistent command history. Empty string disables persistence. |
| `Env` | `[]string` | `nil` | Static extra environment variables (`"KEY=VALUE"`), additive to the current environment. Applied before session env. |
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

Some binaries disable color when stdout is not detected as a TTY. Pass the
binary-specific override via `Config.Env`:

```go
sh := cobrashell.New(cobrashell.Config{
    BinaryPath: "kubectl",
    Prompt:     "k8s> ",
    Env:        []string{"FORCE_COLOR=1"},
})
```

Common overrides:

| Binary | Variable |
|--------|----------|
| Most tools | `FORCE_COLOR=1` |
| kubectl / helm | `KUBECOLOR_FORCE_COLORS=true` |
| gh | `GH_FORCE_TTY=true` |

PTY allocation is deferred to a future release (see [ADR-003](adr/003-no-pty-v1.md)).

## Known limitations (v1)

- **Unix only.** `chzyer/readline` and Unix signal semantics are not portable to Windows.
- **No pipes.** `list | grep foo` is not supported; the input is passed verbatim to the binary.
- **No PTY.** Interactive subcommands (`vim`, `less`, `ssh`) do not work correctly.
- **No aliasing or multi-line input.**

## Architecture

See [DESIGN.md](DESIGN.md) for the full design rationale and [adr/](adr/) for architectural decision records.

```
Tab press → Completer.Do()
              → shlex.Split(line[:cursor])
              → binary __completeNoDesc contextArgs... toComplete
              → parseCompletions() → candidates → readline

Enter → Shell.execute()
          → shlex.Split(line)
          → handleEnvBuiltin() (if EnvBuiltin configured)
          → BeforeExec hook
          → exec.Command(binary, tokens...)  [SIGINT suppressed in parent]
          → AfterExec hook
```
