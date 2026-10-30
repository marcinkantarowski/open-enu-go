# analysis - the guardrails

One `go/analysis` analyzer per rule, one `testdata/` fixture per analyzer that must fail
(run by `go test`), and `make selftest` planting one violation per analyzer in a real
module and expecting `make arch` to refuse it. `cmd/arch` is the multichecker that runs
every analyzer below; `make arch` and `make check` invoke it over both Go modules. A rule
missing from its list is a rule that is not enforced.

| Package | Rule |
|---|---|
| `writeboundary` | in a module, `*scope.Tx` appears only in `handler.go`, `repository.go` and `job.go`, and `scope.Pool` is opened only in `repository.go`; anywhere else it is a write that bypassed the bus and left no audit entry |
| `thincontroller` | a module's `api.go` imports only the kernel's HTTP surface and its own `contract`, and uses nothing declared in `handler.go` or `job.go`; reading through `repository.go` is allowed |
| `rawsql` | `pgx` `Exec`/`Query`/`QueryRow` only in `repository.go`; `scope.Unscoped` only in `repository.go` and only with a non-empty string literal as its reason |
| `platformservice` | no raw Redis client and no `os.WriteFile`/`os.Create`/`O_WRONLY` outside the kernel - storage and cache go through the kernel's contracts |
| `flagdeclared` | every flag key a package reads has a `flags.Definition` literal in the same package; the registry then checks the definition is listed in `Module.Flags` |
| `rawstring` | every constant that ends up as an `i18n.Key` has the dotted-lowercase shape of a catalogue key, so a sentence cannot slip in as a key |
| `impersonationguard` | in a module's `api.go`, a controller method that dispatches a command calls `auth.Guard.DenyImpersonated` first, or carries `//impersonation:allowed <reason>` (ADR-0020) |
| `textlen` | in a module's `api.go` and `handler.go`, `len()` of a string is not compared with a non-zero constant - the spec's `maxLength` and `VARCHAR(n)` count characters (`utf8.RuneCountInString`); a limit that really is in bytes is written `len([]byte(s))` |
| `tableowner` | a SQL string literal in a module names only its own tables (and the kernel's), and its migrations `REFERENCES` only its own - ownership read from every module's `migrations/*.sql` |
| `modpath` | not a rule - the shared helper that answers "is this path the kernel?" and "which module is this in?" for the analyzers above |

Module boundaries and kernel purity are not here: `internal/` packages and the missing
dependency edge make them compile errors. `selftest` still proves both by breaking them.
