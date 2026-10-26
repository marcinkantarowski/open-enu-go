# workflows

`ci.yml` runs the same `make ci` a developer runs, on every pull request, from a clean clone:
native Go (`actions/setup-go`, pinned by `backend/go.mod`), mkcert, `make builddev`, then the
gate - every guardrail, `selftest`, every Go test, route coverage, `typecheck`, `smoke` and the
browser suite. It then refuses a build that changed a committed file (the lockfile, the
generated registry, ACL or spec). Image layers are cached through `docker/compose.ci.yml`;
a push to `main` only warms that cache. A failing run uploads the Playwright report.

`../dependabot.yml` opens grouped update pull requests once a month - both Go modules, npm,
the Dockerfiles, the compose files and these actions - and each one runs this workflow.
`make versions-check` fails such a PR until every file naming that service's version agrees.
