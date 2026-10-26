# cmd/openenu

One binary, subcommands. `openenu` with no arguments, `help` or an unknown command prints the
list; `version` prints the kernel version.

| Subcommand | What it does |
|---|---|
| `api` | serve the API (`-addr`, default `:8080`) |
| `worker` | work the outbox: events, broadcasts, background jobs (River) |
| `scheduler` | a loop running every module's `module.Task`: `EveryMinute` tasks each `SCHEDULER_TICK_SECONDS`, `Daily` tasks at start and then each `SCHEDULER_INTERVAL_SECONDS` |
| `migrate` | apply River's, the kernel's and every module's migrations (goose) as the schema owner, through `DATABASE_ADMIN_URL` |
| `modules` | print what module discovery resolved |
| `health` | probe a running api (`-url`, default from `API_ADDR`) and exit 1 unless the body says ok - the container healthcheck, since the distroless image has no `curl` |
| `user`, `tenant`, `apikey`, `manager`, `attachment` | the console: `user create`, `tenant purge-unverified`, `apikey create`, `manager create`, `attachment sweep-orphans` - each a verb plus flags, thin calls into the module's root package (`console.go`) |
| `inventory` | write `.ai/inventory.json` - what already exists (`-check` fails if stale; `make inventory`, `make arch`) |
| `mcp` | serve the API to an MCP client over stdio as a client holding one scoped key (`-key`, `-url`, `-list`): tools from the fragments + `acl_gen.go`, served with the official Go SDK |
| `route-coverage` | fail if an operation in the spec was never reached by a test - the spec minus the trace `make test` wrote (`-list` prints them all) |

`make module NAME=X` scaffolds a module from `scripts/dev/module.sh`; there is no `module` or
`console` subcommand.

`main.go` wires the registry (`module_registry_gen.go`) into the kernel and dispatches; each
subcommand registers itself from its own file's `init()`. `platform.go` (`newServices`) is
the one place a backing service is chosen from the environment, shared by `api`, `worker`,
`scheduler` and the console.
