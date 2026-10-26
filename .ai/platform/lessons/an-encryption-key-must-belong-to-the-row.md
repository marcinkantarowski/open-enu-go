---
tags: [encryption, tenancy, pgx]
date: 2026-10-25
---
# An encryption key must be a property of the row, not of the ambient context

When a stored value can only be interpreted with a key, the key must be derivable from the
row. A key taken from whatever context is current works whenever the row is read under its
own tenant, and fails everywhere else.

## The trap
`crypto.Encryptor` derives a key per tenant ([ADR-0016](../adr/0016-light-encryption.md)):
`Encrypt(plaintext, tenantID)` and `Decrypt(payload, tenantID)`, with `""` as the global key.
The tempting call passes the ambient tenant:

```go
tenant, _ := scope.TenantID(ctx)
email, err := r.crypto.Encrypt(u.Email, tenant)
```

That is correct for a tenant-scoped row read under its own tenant, and silently broken for
everything else. A user is deliberately **not** tenant-scoped - a person belongs to several
workspaces ([ADR-0010](../adr/0010-membership-from-day-one.md)) - so an email written during
sign-up under one scope and read during login under another, or under none, fails with:

```
crypto: decryption failed - the master key changed, or this row belongs to a different tenant than the current scope
```

An error that offers the master key first, for a bug that has nothing to do with it. GCM
cannot tell a wrong key from tampering, so `ErrDecrypt` cannot say which. The data is intact
and unreadable.

Unit tests pass: they encrypt and decrypt under the same tenant, which is the case that works.
It surfaces only when one request writes under one scope and another reads under a different
one.

## The rule
**The key's owner comes from the row, never from the context.** If the code that encrypts
cannot see the row, it must not guess.

| Row | Key | Here |
|---|---|---|
| belongs to a tenant | the row's `TenantID` | `example_project.client_reference`, a `webhook_endpoint`'s secret |
| belongs to no tenant | `""`, the global key | `identity_user` email and display name, `platform_manager` email and display name |

## Where it is held
- `backend/internal/module/example/internal/repository.go`: `encrypt` is the one place the
  encrypted column is written, "under the row's tenant key, never the ambient scope's"
  (`r.crypto.Encrypt(*p.ClientReference, p.TenantID)`), and `scan` decrypts with
  `p.TenantID`.
- Identity and Manager encrypt with `""`; Webhook encrypts an endpoint's secret with
  `e.TenantID`.
- `backend/kernel/crypto/crypto_test.go` asserts that another tenant's key fails with
  `ErrDecrypt`; `example/project_api_test.go` reads the column straight from the table and
  asserts it holds `v1:` ciphertext and not the plaintext.

## Where else this applies
- Any signed or encrypted artefact whose key varies: storage URLs, export bundles, webhook
  signatures. Ask what the key is derived from, and whether that thing travels with the data.
- A lookup on an encrypted value goes through its sibling `*_hash` column
  (`crypto.HashForLookup`), never the ciphertext, which differs on every write.
