# ADR-0014 - No undo/redo; command snapshots keep it possible

**Status:** accepted · **Date:** 2026-10-22

## Context
Framework-level undo is built on the command pattern: handlers declare an inverse and the
bus issues undo tokens. Valuable, and a large surface - every handler must implement a
correct inverse, and "correct" gets subtle as soon as two people interleave edits.

## Decision
No undo. But every write goes through a command bus whose audit entry carries the before
and after state (ADR-0013).

The *data* undo needs therefore exists from day one; only the inverse operations and the
token machinery are absent. A project that needs undo adds an inverse to the handlers that
warrant it, without re-architecting anything.

## Consequences
- Audit answers "what changed" and "what was it before" - most of the value, none of the
  inverse-correctness problem.
- Mistakes are recovered by a restore (ADR-0026) or by a compensating command, not by undo.
