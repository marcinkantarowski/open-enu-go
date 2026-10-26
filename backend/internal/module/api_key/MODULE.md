# ApiKey - `api_key`

Owns machine access: a key is minted once with a subset of the permissions modules declare,
stored as a SHA-256 hash with a 12-character prefix a person can recognise, and revoked
rather than deleted. It is the kernel's `auth.KeyResolver`: a bearer `sk_...` becomes a
principal that **is** the key - no person, no session, exactly the key's permissions. It
does not decide who may mint keys (Identity's roles: owners only).

> Budget: 8 KB.

## Surface

- API (`openapi.json`): `GET /api/api-keys`, `POST /api/api-keys` (201, the secret once),
  `DELETE /api/api-keys/{id}` → `{status: "revoked"}`
- Permissions: `api_key.view`, `api_key.manage`
- Commands: `api_key.created` (snapshot has the prefix, never the hash), `api_key.revoked`
- Tables: `api_key` (RLS; the resolver's lookup by hash is the one `Unscoped` read)
- Console: `openenu apikey create --tenant <id|slug> --name --permission ... [--days]`

## Public contract

Nothing under `contract/`. The root package exports `Module.Keys` (through the kernel's
`module.Module`), `CreateKeyInput` and `CreateKey` for the console.

## Notes for agents

- A key for a permission no module declares is refused (400 naming it). A key never
  outranks its tenant: a suspended tenant's keys stop authenticating.
- Unknown, revoked and expired keys answer the same 401 as a bad JWT. The resolver returns
  "not found" for all three; the middleware does the rest.
- `last_used_at` is written on every authenticated request, in the key's own tenant scope.

## Depends on

`tenant` (`contract.Reader` - a key's tenant must be active).
