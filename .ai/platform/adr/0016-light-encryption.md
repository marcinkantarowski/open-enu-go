# ADR-0016 - Field encryption with derived per-tenant keys, not a KMS

**Status:** accepted · **Date:** 2026-10-22

## Context
Per-tenant data keys held in an external key manager are correct for an enterprise
deployment, and they make that service a prerequisite for booting - which contradicts this
foundation's rule that it starts with nothing external.

## Decision
`crypto.Encryptor` encrypts a column with AES-256-GCM. The data key is **derived**, by
HKDF-SHA256 from `APP_ENCRYPTION_KEY` with the tenant's id as salt, so no key material is
stored per tenant. A row that belongs to no tenant - a person, who spans tenants - is
encrypted under the key derived for no tenant.

An encrypted value differs on every write, so it is never queried, sorted, indexed or put
in the search index. Equality lookups go through a sibling `*_hash` column filled with
`crypto.HashForLookup` (SHA-256 of the trimmed, lower-cased value): `identity_user.email` is
ciphertext, and login queries `email_hash`.

Key derivation sits behind `crypto.KeyProvider`, with one implementation (`DerivedKeys`), so
a key-manager-backed provider is a type, not a refactor.

## Consequences
- Boots with nothing external; encryption is on by default rather than aspirational.
- `APP_ENCRYPTION_KEY` is catastrophic to lose. It travels in the encrypted secrets bundle
  of every backup (ADR-0026), and is never regenerated on a server that has one.
- Encrypted columns cannot be searched or sorted in SQL; `*_hash` covers equality only.
- A key must belong to the row: an encrypted column on a row whose tenant can differ from
  the ambient scope becomes unreadable
  ([`lessons/an-encryption-key-must-belong-to-the-row.md`](../lessons/an-encryption-key-must-belong-to-the-row.md)).
- There is no master-key rotation command. Rotating means re-encrypting every column under
  a new master, a job a project writes when it needs one.
