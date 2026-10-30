# scripts

Every entry point is a Make target; these are what the targets run. `lib/` is sourced, never
executed.

- `lib/` - `log` (log_*, die, quiet), `os` (`sed_inplace`), `require` (require_tool,
  want_tool), `secrets`, `envgen`, `compose` (reads `COMPOSE_FILE`), `stage`, `go` (finds the
  toolchain in PATH, `~/.local/go` or `/usr/local/go`), `tools`.
- `dev/` - the inner loop and the stack: `check.sh`, `check-tools.sh`, `selftest.sh`,
  `module.sh` (`make module`), `manager-module.sh` (`make manager-module`), `init.sh` (names the project: the manifest, the Go module path and the defaults, never the kernel),
  `check-docs.sh`, `check-typography.sh`, `agents-budget.sh`, `check-shell.sh`,
  `check-prod-compose.sh`, `check-ports.sh`, `check-route-coverage.sh` (reads the trace
  `make test` wrote), `check-dead-code.sh` (unused TypeScript and unreachable application Go
  are blocking; unreached kernel exports advisory), `deps.sh`, `certs.sh`, `hosts.sh`,
  `edge.sh`, `urls.sh`, `wait.sh`, `smoke.sh`, `typecheck.sh`, `e2e.sh` (the browser
  tests on their own database), `e2e-fixtures.sh` (the accounts they log in as, through
  `openenu user create` and `manager create`), `db-reset.sh`.
- `remote/` - `preflight.sh` (read-only), `dns-check.sh`, `dns-emit.sh`, `deploy-key.sh`,
  `provision.sh`, `install-units.sh`, `release.sh` (ships the one static binary's image),
  `deploy.sh`, `promote.sh`, `stack.sh`, `urls.sh`, `backup.sh`, `backup-verify.sh`,
  `restore.sh`.

Scaffolding is `module.sh`; the inventory is `openenu inventory` behind `make inventory`.
