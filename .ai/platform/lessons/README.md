# Platform lessons

One file per lesson, written when a mistake is *understood* - not when it is made. Each states
the trap, the rule, and where in this repository the rule is held.
Read the index; open only the rows that match the task. Never bulk-read this directory.

| Lesson | When it bites |
|---|---|
| [a-selftest-runs-the-copys-scripts.md](a-selftest-runs-the-copys-scripts.md) | A dev script resolves `ROOT` from its own path; running the real tree's `init.sh` with the copy as cwd renames the real repository |
| [sentinel-strings-must-not-be-renameable.md](sentinel-strings-must-not-be-renameable.md) | Writing any tool that rewrites the tree it lives in - `init`, `platform-update`, `make module` templates |
| [status-codes-alone-are-not-a-health-check.md](status-codes-alone-are-not-a-health-check.md) | Writing any smoke test, health endpoint or deploy gate |
| [a-generator-must-validate-its-own-output.md](a-generator-must-validate-its-own-output.md) | Writing anything that emits code - scaffolders, codemods, generators |
| [a-guardrail-firing-on-your-own-code-is-a-decision-point.md](a-guardrail-firing-on-your-own-code-is-a-decision-point.md) | Any new analyzer - it WILL catch something legitimate on its first run |
| [an-encryption-key-must-belong-to-the-row.md](an-encryption-key-must-belong-to-the-row.md) | Encrypting a column, signing an artefact, or anything whose key varies by context |
| [test-contracts-against-the-real-implementation.md](test-contracts-against-the-real-implementation.md) | Whenever a collaborator constrains the values you pass it - caches, storage, transports |
| [a-layers-config-is-the-apps-config.md](a-layers-config-is-the-apps-config.md) | Setting anything path-shaped in a Nuxt layer - a module's `nuxt.config.ts` |
| [a-no-op-write-does-not-bump-the-version.md](a-no-op-write-does-not-bump-the-version.md) | Testing optimistic locking, `If-Match`, or anything that asserts "the second writer is refused" |
| [a-test-client-never-asks-for-permission.md](a-test-client-never-asks-for-permission.md) | Any behaviour the BROWSER enforces - CORS, cookie attributes, SSE credentials, exposed headers |
| [a-mechanism-with-no-way-in-is-untested-by-construction.md](a-mechanism-with-no-way-in-is-untested-by-construction.md) | Building anything read far more often than written - flags, settings, registries, catalogues |
| [a-handler-cannot-record-its-own-failure.md](a-handler-cannot-record-its-own-failure.md) | Writing anything that must survive a failure - delivery logs, retry counters, error reports |
| [an-ambient-dependency-breaks-its-legitimate-callers.md](an-ambient-dependency-breaks-its-legitimate-callers.md) | Reading the tenant from scope on a write; adding anything that runs for every write |
| [a-worker-runs-the-code-it-started-with.md](a-worker-runs-the-code-it-started-with.md) | Adding or editing a job, a subscriber or a clock task - anything in a long-lived process |
| [a-class-nobody-scans-is-a-style-nobody-gets.md](a-class-nobody-scans-is-a-style-nobody-gets.md) | Adding a component to `ui-kit`, or anything looking unstyled with no error anywhere |
| [a-dev-script-runs-on-hosts-it-was-not-written-on.md](a-dev-script-runs-on-hosts-it-was-not-written-on.md) | Writing any shell under `scripts/dev` or `scripts/lib` |
