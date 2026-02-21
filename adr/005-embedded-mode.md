# ADR-005: Embedded Mode Design

**Date:** 2026-02-21
**Status:** Accepted

## Context

The primary (subprocess) mode treats the wrapped binary as a black box and discovers its structure via `__completeNoDesc`. This works for any Cobra binary with zero code changes, but has a fundamental limitation: every command invocation and every tab-completion request spawns a new process, so state cannot be shared across commands.

The embedded mode addresses this by running the Cobra command tree **in the same process** as the shell. This is the pattern used by `go-cs-metrics` (the original motivation for this library) and similar tools that need:

- Shared in-process state (DB handles, caches, auth tokens)
- Dynamic completions sourced from live data that is only available inside the process
- Zero subprocess overhead per command

Three design questions arose during implementation:

1. How should completion work in-process?
2. How should flag state be managed between commands?
3. Should `EmbeddedHooks` be the same type as `Hooks`?

---

## Decision 1: In-process completion via command tree traversal

**Rejected alternative:** Redirect the root command's output to a `bytes.Buffer`, invoke `__completeNoDesc` via `Execute()`, and parse the result with `parseCompletions`. This would reuse the existing parser but has problems:
- `__completeNoDesc` in Cobra writes to `cmd.OutOrStdout()` on the *completion* command's subtree, not `rootCmd.OutOrStdout()`. Redirecting root's output does not capture completion output reliably without navigating to the completion sub-command and setting its output directly.
- It would also suppress any legitimate stdout from the `__completeNoDesc` command (e.g. debug output) which might be helpful in testing.

**Chosen approach:** Walk the command tree directly using `cobra.Command.Traverse(contextArgs)`, which returns the deepest matching command and the remaining unmatched args. From there, completion candidates are gathered from three sources in order:

1. Static subcommand names (via `cmd.Commands()`, skipping hidden ones).
2. `EmbeddedConfig.DynamicCompletions[cmd.Name()]` — library-registered live completion functions.
3. `cmd.ValidArgsFunction` — cobra's native per-command completion mechanism.

Flag names are collected from `cmd.Flags()` and `cmd.InheritedFlags()` (which handles persistent flags from ancestor commands) when the partial word starts with `-` or when no positional candidates exist.

This approach is fast (no I/O), correct, and integrates with cobra's own completion extension points.

**Limitation:** Flag *value* completion is not implemented. When the user has typed `--port `, no candidates are offered for the port value. This matches the `--help` fallback limitation and is acceptable for v1.

---

## Decision 2: Reset flag state between executions

cobra's `FlagSet` stores flag values in variables bound at command definition time (e.g. via `Flags().IntVar(&port, ...)`). When `Execute()` is called, cobra parses the provided args and updates those variables. Flags absent from the args are **not** reset — they retain whatever value they had from the previous invocation.

This means that if the user runs `serve --port 9090` and then `serve`, the port variable still holds 9090 on the second run, rather than the default 8080.

`resetCommandTree` traverses the full command tree before each `Execute()` call and, for every flag whose `Changed` marker is true (indicating it was set on the command line), calls `f.Value.Set(f.DefValue)` and clears `Changed`. This restores the default value as cobra would see it on a fresh process invocation.

**Trade-off:** `Set` calls the flag type's string parser (`strconv.ParseInt`, etc.), which is slightly expensive. For typical command trees with tens of flags, the cost is negligible. Deeply nested trees with thousands of flags could see measurable overhead; this is noted as a future optimisation target if it becomes an issue.

---

## Decision 3: Separate `EmbeddedHooks` type

`Hooks.OnStart` has the signature `func(shell *Shell)`. `*Shell` is the subprocess-mode shell type and is meaningless in embedded mode — it has no binary path, no subprocess machinery, and different exported methods.

Options considered:

| Option | Pros | Cons |
|--------|------|------|
| Reuse `Hooks`, pass `nil` for OnStart | No new type | Silent nil in OnStart hook; documentation hack |
| Change `Hooks.OnStart` to `func()` | Simple | Breaks subprocess-mode hooks API |
| Separate `EmbeddedHooks` with `OnStart func(*EmbeddedShell)` | Type-safe, correct | New type to learn |

`EmbeddedHooks` was chosen. It mirrors `Hooks` exactly except `OnStart func(*EmbeddedShell)`. The two shell types are intentionally not unified under a common interface because their operational differences (subprocess vs in-process, binary path vs command tree) make a shared interface misleading.
