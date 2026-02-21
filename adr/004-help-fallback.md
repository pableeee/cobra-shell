# ADR-004: `--help` Parsing as Fallback Completion

**Date:** 2026-02-21
**Status:** Accepted

## Context

`__completeNoDesc` is only available in Cobra ≥ 1.2. Binaries built with older versions of Cobra, or with frameworks other than Cobra entirely, will exit non-zero when that hidden command is invoked. Without a fallback, the shell would offer no tab completion at all for these binaries — degrading to a plain readline loop with history only.

`--help` is nearly universal: every CLI tool that is worth wrapping in an interactive shell provides it, and its output reliably lists available subcommands and flag names in a human-readable format. Parsing it heuristically recovers partial completion coverage for the common case.

## Decision

When `__completeNoDesc` exits non-zero, fall back to calling `binary [contextArgs...] --help` and parsing its stdout with `parseHelp`. The fallback:

1. Identifies the section in play by looking for unindented lines matching `"Available Commands:"`, `"Commands:"`, `"Flags:"`, or `"Global Flags:"`.
2. Extracts the first whitespace-delimited token from each indented line in a commands section.
3. Extracts the first `--`-prefixed token from each indented line in a flags section.
4. Performs client-side prefix filtering against `toComplete` (unlike `__completeNoDesc`, which filters server-side).

The fallback returns directive `0` (Default), which readline treats the same as a successful but file-fallback-allowed `__completeNoDesc` result.

## Consequences

**Limitations:**
- Only the default Cobra help template is reliably supported. Custom templates, ANSI colour codes in output, or non-standard indentation may produce incomplete or empty results.
- Flag *values* cannot be completed — only flag *names*. A user typing `--port ` will see no completions.
- Every Tab press for a non-Cobra binary spawns one `--help` subprocess. This is not cached; repeated completions on the same subcommand re-parse the same output. Caching is left for a future improvement.
- Short flags (`-h`) are not captured; only long flags (`--help`). Short flags are single characters and users rarely tab-complete them.

**Graceful degradation:** if `--help` also fails (binary exits non-zero, produces no parseable output, or times out), `parseHelp` returns an empty slice and the shell shows no completions — the same behaviour as having no fallback at all. It never crashes or surfaces an error to the user.
