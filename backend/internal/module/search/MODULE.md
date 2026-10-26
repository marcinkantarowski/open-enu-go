# Search - `search`

Owns the one endpoint over every module's searchable declarations (`Module.Search`,
collected as `svc.Searchable`). No table: the index is the kernel's (`search_index`,
kernel migration 0002) and the writes are each module's, in its own handlers. What this
module adds is the rule the kernel cannot enforce alone - **a type's permission is checked
before the query runs**, per type, because an excerpt is content.

> Budget: 8 KB.

## Surface

- API (`openapi.json`): `GET /api/search?q=` → `{items: [{type, id, score, excerpt}],
  meta: {query, total}}`; at most 20; an empty or blank `q` is an empty result, never
  "everything"
- Permissions: none of its own - the declaring modules' (`example.view` for
  `example.project`), one per entity type. Only the types whose permission the caller
  holds are queried at all; a caller holding none gets an empty result, not a 403
- Realm: tenant only (`auth.RequireRealm`); the index is scoped by RLS like every table

## Public contract

Nothing under `contract/`.

## Notes for agents

- To make a module's data findable, declare it on *that* module (`Search:
  internal.Searchable`) and index from its handlers - never touch this one.
- `score` is rounded to four places so a response is stable across runs.

## Depends on

Nothing.
