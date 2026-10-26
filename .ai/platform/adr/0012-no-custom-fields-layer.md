# ADR-0012 - No runtime custom-fields or custom-entities layer

**Status:** accepted · **Date:** 2026-10-22

**Read this before proposing an EAV layer.**

## Context
Some platforms let administrators define fields and entities from the UI with no migration,
backed by entity-attribute-value storage plus an index that flattens it into read tables.
That is the right bet for a no-code product sold to enterprises. It is the wrong one here:

1. A migration takes minutes to write, and an agent writes it correctly.
2. EAV plus a flattening index is a large, permanently load-bearing subsystem - exactly what
   this foundation exists to avoid carrying by default.
3. Typed columns are what make `openapi.json` and the generated frontend types useful
   (ADR-0006). Runtime fields are invisible to both.

## Decision
No custom-fields layer. Schema changes are migrations.

**Escape hatch:** an entity that genuinely needs open-ended per-tenant data gets an
`attributes JSONB NOT NULL DEFAULT '{}'` column, documented in its `MODULE.md`, with a GIN
index once something queries inside it. The `Example` module's `Project` has one.

## Consequences
- Non-developers cannot add fields. Accepted - this is a developer foundation.
- A project that genuinely needs no-code schema editing should choose a platform built
  around one rather than grow one here.
