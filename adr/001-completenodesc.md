# ADR-001: Use `__completeNoDesc` Instead of `__complete`

**Date:** 2026-02-21
**Status:** Accepted

## Context

Cobra injects two hidden completion commands into every binary it builds:

- `__complete` — returns one candidate per line, with an optional tab-separated description appended to each (`subcommand\tDoes the thing`), followed by a directive line (`:N`).
- `__completeNoDesc` — identical except descriptions are stripped server-side before output.

Both commands accept the same arguments (all tokens typed so far, last token being the partial word) and return the same `ShellCompDirective` bitmask on the final line.

The readline library (`github.com/chzyer/readline`) displays a flat list of completion candidates with no mechanism to render per-candidate descriptions. Any description text present in the output would need to be stripped client-side before passing candidates to readline.

## Decision

Use `__completeNoDesc` exclusively for all tab completion requests.

## Consequences

- No client-side description stripping is needed; output lines map directly to candidates.
- The `\t` tab separator in `__complete` output is never parsed, eliminating a class of parsing edge cases (e.g., descriptions that themselves contain colons or special characters).
- Descriptions are permanently unavailable to the UI. If a future readline replacement supports inline descriptions, this decision will need revisiting.
- Binaries that do not implement `__completeNoDesc` (pre-Cobra or non-Cobra binaries) will return an error exit code or unknown-command output; `parseCompletions` treats a missing directive line as zero candidates, which degrades gracefully to no completion rather than broken output.
