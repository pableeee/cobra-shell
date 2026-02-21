# ADR-003: No PTY Allocation in v1

**Date:** 2026-02-21
**Status:** Superseded by [ADR-007](007-pty.md)

## Context

When the shell spawns a subprocess, the subprocess's stdout is inherited from the parent process. Many CLI tools detect whether their stdout is a TTY and, if not, disable colored output (e.g., `git`, `kubectl`, `gh`). Because the shell's own stdout is a TTY but the subprocess's detection may disagree, some commands produce uncolored output inside the shell even though they would produce colored output when invoked directly.

Allocating a PTY (pseudo-terminal) for each subprocess via `github.com/creack/pty` would solve this: the subprocess would believe it is writing to a real terminal and enable color. This is how tools like `script(1)` and many terminal multiplexers work.

However, PTY allocation introduces several complications:

1. **Pager activation.** Commands like `git log` or `kubectl describe` detect a TTY and launch an interactive pager (`less`, `more`). Inside a PTY this pager launches but the shell cannot control it correctly, leading to broken display.

2. **Signal forwarding complexity.** With a PTY, SIGINT, SIGWINCH, and other terminal signals must be manually forwarded from the outer terminal to the PTY. The existing signal model (notify parent, child receives SIGINT via process group) no longer applies.

3. **Windows incompatibility.** PTY support on Windows requires a separate implementation path and is explicitly out of scope for v1 (see ADR-003 companion note in DESIGN.md).

4. **Increased dependency surface.** `github.com/creack/pty` is non-trivial and pulls in OS-specific code.

## Decision

No PTY allocation in v1. Subprocesses inherit stdin/stdout/stderr from the shell process directly.

**Workaround for color output:** users can set `FORCE_COLOR=1` (or the binary-specific equivalent, e.g., `CLICOLOR_FORCE=1`) via `Config.Env`:

```go
cobrashell.New(cobrashell.Config{
    BinaryPath: "kubectl",
    Env:        []string{"FORCE_COLOR=1"},
})
```

Most modern CLIs respect at least one of these variables.

## Consequences

- Some commands will produce uncolored output inside the shell even though they produce colored output when run directly.
- Interactive subcommands that require a real TTY (e.g., `vim`, `less`, `ssh`) will not work correctly. This is documented as out of scope.
- The implementation stays simpler and portable.
- PTY support can be added in a later milestone behind an opt-in `Config` flag without breaking the existing API.
