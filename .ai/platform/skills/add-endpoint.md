# Skill - add an endpoint

The spec is written first and the server is generated from it
([ADR-0006](../adr/0006-spec-first-openapi.md)), so an endpoint starts as JSON and becomes a
Go method the compiler insists on. Each step below fails the build when skipped.

---

## 1. The operation, in the module's fragment

In `backend/internal/module/<name>/openapi.json`:

- the path, the method, request and response schemas, and every error status it answers;
- `operationId` `<verb>_<module>_<entity>_<action>` - `get_invoicing_invoice_show`. Route
  coverage, the inventory and the MCP tool list all key on it.

```bash
make generate     # the module's api.gen.go, the registry, backend/openapi.json
```

The strict server interface now has a method the module does not implement: the build
fails until it does.

## 2. A thin method in `internal/api.go`

```go
func (a api) GetInvoicingInvoiceShow(ctx context.Context, req GetInvoicingInvoiceShowRequestObject) (GetInvoicingInvoiceShowResponseObject, error) {
	if _, err := auth.Require(ctx, "invoicing.view"); err != nil {
		return nil, err
	}
	inv, err := a.repo.get(ctx, req.Id)
	...
}
```

- The permission is the string literal in `auth.Require`, declared in `Module.Permissions`.
  `tools/genacl` reads it from here into `cmd/openenu/acl_gen.go`, so the MCP catalogue
  offers exactly what the server enforces.
- Validate, delegate, answer. No `*scope.Tx`, no other module's `internal/` - `thincontroller`
  and `writeboundary` refuse them. Text limits in characters: `utf8.RuneCountInString`
  (`textlen`).
- Another tenant's record is **not found**, never forbidden: row-level security removes it
  from the query, and a 403 would confirm that the id is real.

## 3. Writes: a command and a handler

```go
inv, err := command.Dispatch[invoice](ctx, a.bus, issueInvoice{...})
```

The handler gets the transaction; `tx.Update(old, new)` records the before/after snapshot
([ADR-0013](../adr/0013-command-bus-with-explicit-snapshots.md)). For a versioned row, take `If-Match`
and answer a stale write with the 409 carrying both versions. A destructive action calls
`a.guard.DenyImpersonated` first.

## 4. What it returns

- `201` with the record for a create; `202` when the work is queued, not done.
- Errors are `httpx.BadRequest(key)`, `httpx.NotFound(key)`, `httpx.Forbidden(key)` - the
  kernel shapes the envelope. Never write the body by hand.
- Reject unknown or unsupported input rather than ignoring it. A setting that "saves" and
  then does not apply is a bug report nobody can reproduce.
- Decide per endpoint what leaves the server. An encrypted column belongs in `show`, not in
  a list.

## 5. A test - before you call it done

In the module's test (`<name>_test.go` from the scaffold), through the real middleware and a real database: the
answer's content, then the refusals - where the value is: another tenant's id, a member
without the grant, a missing field, a replayed token.

```bash
make test route-coverage    # the operation must appear as reached
```

## 6. The types

```bash
make types
```

`ui-kit/types/api.d.ts` is generated from `backend/openapi.json` and stamped with its hash;
`make gen-check` fails when it is stale.

## 7. Strings

Every user-facing string is a key, in `en` **and** `pl`, in the module's
`i18n/messages.*.json` - and, for what a screen shows, in its frontend layer's
`i18n/locales/{en,pl}.json`. The registry refuses a backend key in one locale only,
`make i18n-check` refuses one on either side, and `rawstring` refuses a sentence passed as
a key ([`i18n.md`](../docs/i18n.md)).

---

## Done when

```bash
make check && make ci
```

- [ ] operation named `<verb>_<module>_<entity>_<action>`, guarded by a declared permission
- [ ] `api.go` is thin; every write is a command
- [ ] a test asserting content **and** at least one refusal
- [ ] `make route-coverage` counts it
- [ ] `backend/openapi.json` and `ui-kit/types/api.d.ts` regenerated
- [ ] no hard-coded user-facing string
