# ADR-006: Session Environment Variable Management

**Date:** 2026-02-21
**Status:** Accepted

## Context

Many CLI tools use environment variables for configuration (API credentials,
endpoint URLs, feature flags). When wrapping such a binary in cobra-shell,
users typically need to export these variables in the parent shell before
launching cobra-shell, or pass them via `Config.Env`. Neither approach allows
the values to be changed at the prompt without restarting the shell.

Three design questions arose:

1. How should session env be stored and merged with the process environment?
2. How should subprocess mode expose the feature to end users?
3. Should embedded mode support session env?

---

## Decision 1: In-memory store, last-value-wins merge at spawn time

Session environment variables are stored in a `map[string]string` on `Shell`.
At subprocess spawn time (for both command execution and tab-completion via
`__completeNoDesc`), `buildEnv()` layers three sources in ascending priority:

1. `os.Environ()` — inherited process environment.
2. `Config.Env` — static additive variables from library configuration.
3. `sessionEnv` — variables set at runtime via `Shell.SetEnv`.

Because `os/exec` uses the **last occurrence** of a key in `Cmd.Env`,
appending in this order gives session values highest precedence without
requiring deduplication.

`os.Setenv` is **never called**. Session env changes are strictly scoped to
subprocesses spawned by cobra-shell; the current process environment and any
other goroutines are unaffected.

---

## Decision 2: Opt-in named built-in for subprocess mode

The feature is exposed to end users as a built-in command whose name is
configured via `Config.EnvBuiltin` (default `""`, disabled). This avoids
namespace conflicts with binaries that have their own subcommand of the same
name (e.g. `heroku env`).

When `EnvBuiltin` is non-empty, `execute()` intercepts any line whose first
token matches the configured name **before** calling `BeforeExec` or spawning
the binary. The built-in supports three subcommands:

| Subcommand | Effect |
|------------|--------|
| `list` | Print current session env as `KEY=VALUE` lines |
| `set KEY VALUE` | Add or overwrite a session variable |
| `unset KEY` | Remove a session variable |

Tab-completion for the built-in is handled in `completer.Do()` before the
normal `__completeNoDesc` path: subcommand names (`list`, `set`, `unset`) are
offered after the built-in name; current session keys are offered after
`unset`.

**Alternative considered:** Hard-coding `env` as a reserved keyword. Rejected
because it would break binaries that use `env` as a real subcommand.

---

## Decision 3: Embedded mode excluded

In embedded mode (`EmbeddedShell`), commands run in-process via
`cobra.Command.Execute`. Setting variables with `os.Setenv` would pollute the
shared process environment, affecting all goroutines — a side effect that
conflicts with the isolation goal stated in ADR-005.

In-process commands read env via `os.Getenv`, which cannot be scoped
per-session without process-level mutation. Therefore `EmbeddedShell` does not
expose session env management. Users who need env-backed configuration in
embedded mode can manage it through their own cobra command tree or via
application-level state.
