# ADR-002: Use `github.com/google/shlex` for Tokenization

**Date:** 2026-02-21
**Status:** Accepted

## Context

Both the Completer and the Executor need to split a raw input line into tokens before passing them to the binary. The splitting must correctly handle:

- Single-quoted strings (`'foo bar'` → one token `foo bar`)
- Double-quoted strings (`"foo bar"` → one token `foo bar`)
- Backslash escapes (`foo\ bar` → one token `foo bar`)
- Trailing whitespace (does not produce an empty token)

Rolling a custom tokenizer would cover the common cases but is prone to subtle bugs at edge cases (nested quotes, backslash inside single quotes, embedded newlines).

Alternatives considered:

| Option | Pros | Cons |
|--------|------|------|
| Custom minimal parser | No dependency | High edge-case risk; maintenance burden |
| `strings.Fields` | Zero dependency | No quote handling at all |
| `github.com/google/shlex` | POSIX-ish, battle-tested, tiny | External dependency |
| `mvdan.cc/sh/syntax` | Full POSIX shell parser | Far more than needed; pulls in large dependency |

## Decision

Use `github.com/google/shlex` in both the Completer and the Executor.

Using the **same library in both places** is important: a command entered interactively tokenises identically to the way the Completer tokenises the same partial input. Divergent tokenisers would cause completions to suggest candidates that the Executor then fails to reconstruct correctly.

## Consequences

- `github.com/google/shlex` becomes a required dependency.
- The package is small (~300 lines), has no transitive dependencies, and has been stable since 2019.
- Tokenization is POSIX-ish but not strictly POSIX — for example, process substitution (`<(...)`) is not supported, but that is irrelevant since we are not implementing a general shell.
- Unclosed quotes return an error from `shlex.Split`; the Completer responds with no candidates and the Executor prints a parse error and skips execution.
