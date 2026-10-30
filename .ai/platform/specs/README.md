# Platform specs

One spec per non-trivial piece of work on the platform, written before the work and named
`{YYYY-MM-DD}-{kebab-title}.md`: the problem, the approach, the tests, and a changelog that
ends with what was actually done. A project built from this platform writes its own specs in
[`../../specs/`](../../specs/README.md), never here ([ADR-0027](../adr/0027-platform-and-project-knowledge-are-separate-trees.md)).

What the platform is today is described in [`../ARCHITECTURE.md`](../ARCHITECTURE.md), why in
[`../adr/`](../adr/README.md); a spec records one change to it.

| Spec | Summary | Status |
|---|---|---|
| [Production's data on a development machine](2026-10-30-copy-prod-data.md) | `make copyproddata`: the database re-encrypted with the dev key, nothing copied reaches the outside | done |
