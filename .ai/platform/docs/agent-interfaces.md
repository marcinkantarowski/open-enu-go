# Agent interfaces

Two of them, for two different jobs: reading the codebase, and using the running product.

| Job | Interface |
|---|---|
| "Does something like this already exist?" | [`.ai/inventory.json`](../../inventory.json) |
| "Call the API on a customer's behalf" | `openenu mcp` - MCP over stdio |

---

## The inventory

A committed, machine-readable index of every module and what it declares: entities,
commands, the `contract/` surface, tables, permissions, flags, tasks, searchable types,
notifications and routes.

It exists for one failure mode - **reinvention**. A second date formatter, a third way to
page a list, written because finding the first one meant already knowing its name. A
directory listing does not answer "is there already something for this?"; a flat index of
every declared name does.

```bash
make inventory         # regenerate it
make inventory-check   # fail if the committed copy is stale (runs in `make arch`)
```

Committed rather than generated on demand, because the point is to be *in context before
anything is written*. A file an agent has to run a command to produce is a file it will not
have.

It is built by `kernel/inventory` from the registry (what `var Module` declares) and the
module directories (`go/parser` over `internal/entity.go`, `internal/command.go` and
`contract/*.go`; `CREATE TABLE` over the migrations) - never from a running process. An
inventory that can only be produced by a container that boots is missing exactly when
something is broken. No Docker: `make inventory-check` runs in `arch` on a bare machine.

### The similarity hint

`make inventory` also prints pairs of names close enough to be worth a look -
`InvoiceFormatter` beside `InvoiceFormater`. This is **advisory, permanently**. Two helpers
doing the same job rarely share a signature and often do not share a name, so it can only
ever be a hint; a blocking version would be wrong often enough to get muted, which costs
more than the duplicates it would catch.

---

## The MCP server

```bash
# 1. a scoped credential - never an ambient one
docker compose exec -T api go run ./cmd/openenu apikey create --tenant acme --name agent \
  --permission example.view --permission example.manage

# 2. see what a client would be offered
make mcp-list
```

Point a client at it:

```json
{
  "command": "docker",
  "args": ["compose", "exec", "-T", "api", "/openenu", "mcp", "-key", "sk_…"]
}
```

In the dev stack the binary runs under air, so the path is `go run ./cmd/openenu mcp` there;
in production it is `/openenu`. It speaks MCP over stdio through the official Go SDK:
`initialize`, `tools/list`, `tools/call`.

### What it offers, and what it deliberately does not

Tools are **generated** from three sources, each contributing what only it knows:

- the modules' `openapi.json` fragments - which operations are *published*
  ([ADR-0006](../adr/0006-spec-first-openapi.md)). A route that exists but is not in the
  spec is not offered.
- `cmd/openenu/acl_gen.go` - the permission each handler requires, written by `tools/genacl`
  from the `auth.Require(ctx, "…")` literal in the handler's own `api.go`. One place to write
  it, and the generated map cannot disagree with what the server enforces; `make gen-check`
  fails when it is stale.
- the registry - whether a module declares that permission.

Only operations guarded by a **declared module permission** are offered, and that one rule
does all the filtering. An API key is a scoped tenant credential: it cannot sign in, cannot
refresh a browser session, and is refused by the operator realm, because the principal it
resolves to is in the `tenant` realm
([ADR-0019](../adr/0019-realm-isolation-by-audience.md)). Offering those endpoints would
only produce an agent that spends its turns discovering which of its tools are permanently
401.

One exception the rule cannot see: an operation that carries a permission and still refuses
every key, whatever its permissions - Identity's `patch_profile_update` answers a key 403
`identity.error.not_a_person`, because a key has no profile. Such an operation says so on
itself in the module's `openapi.json` with `"x-mcp": false` (read by `kernel/spec`), and
the catalogue skips it. Mark an operation the moment its controller refuses `p.APIKey`.

The server calls the API over HTTP rather than dispatching in-process. That is the whole
point: the agent is a *client*, and goes through the same middleware, tenant scope and
permission checks as any other machine caller. An in-process version would hand an agent
more access than the credential it was given.

A 4xx comes back as tool content with `isError: true`, not as a protocol error - the model
needs to *read* the refusal. "You lack `example.manage`" is the most useful sentence it can
get.

### The known gap

A tool's `body` is a free-form object. The fragments do carry request schemas, but a schema
repeated in the tool would be validated in the client, where the reason is invisible; the
server's own 400 names the field. Path parameters are required arguments; a read takes a
free-form `query`.

`backend/cmd/openenu/mcp_test.go` holds the generation to its promises against the real
registry and fragments; `backend/kernel/mcp/mcp_test.go` drives the server end to end over
the SDK's in-memory transport against a fake API.

---

## The checks an agent should know about

| Command | What it is for |
|---|---|
| `make check` | the inner loop, under 60s - run after every edit |
| `make ci` | the gate - everything, including the browser suite |
| `make arch` | every architectural guardrail on its own |
| `make route-coverage` | every operation in the spec was reached by a test - reads the trace `make test` wrote |
| `make dead-code` | unused TypeScript and unreachable application Go (blocking); unreached kernel exports (advisory) |
| `make selftest` | proves the guardrails fail when broken |
| `make modules` | what module discovery actually resolved |

**Route coverage is measured, not grepped.** With `ROUTE_TRACE` set, the kernel's request
chain appends the matched pattern of every request to a file (`kernel/httpx/trace.go`);
`make test` sets it, and `openenu route-coverage` subtracts the file from the spec. An
operation no test reached fails the build, unless its fragment says
`"x-no-test-required": "why"` - reported as exempt, never counted as covered.

**Dead code** is `golang.org/x/tools/cmd/deadcode` over both Go modules at once from the
application's roots, plus knip for TypeScript. A finding under `internal/` or `cmd/` fails
the build; a kernel export this application does not reach is printed and does not, because
the kernel is a library with an upgrade contract and the next project built on it reaches
things this one does not. Generated files are not reported. The application is found by the path
in `backend/go.mod`, so it is checked under the project's own name after `make init`, and
`make selftest` proves it there.

The rule underneath all of them: **when you add a convention, add the check that fails when
it is broken, in the same change.** A convention without a check is gone in three months.
