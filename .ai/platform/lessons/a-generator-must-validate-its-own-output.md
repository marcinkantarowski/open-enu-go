---
tags: [generators, tooling, module]
date: 2026-10-25
---
# A generator must validate what it emits

A generator's output is covered by nothing that checks the repository: linters, analyzers and
tests all run on committed code, and this code did not exist until the generator ran. The
generator is the last place that can check it cheaply and the only place that can attribute
a failure correctly.

## The trap
`scripts/dev/module.sh` writes its templates with unquoted heredocs (`cat > file <<EOF`),
because they interpolate `$NAME`, `$STUDLY`, `$KEBAB` and `$MODPATH`. So every `$` and
backtick the target language needs is escaped by hand:

```bash
cat > "$BACK/openapi.json" <<EOF
          "default": { "\$ref": "#/components/responses/Error" }
EOF
cat > "$BACK/${NAME}_test.go" <<EOF
		Module string \`json:"module"\`
EOF
```

One missed backslash is not a shell error. `$$` becomes the shell's PID and a backtick pair
runs its contents as a command and splices in whatever it printed; the file is written either
way. A generator that checks nothing reports success, and the failure surfaces commands later,
in a file nobody wrote by hand, with a message that says nothing about the template - or not
at all, when the damaged file still compiles and simply behaves differently.

## The rule
**A tool that writes code validates the code it writes, before reporting success** - and
validates behaviour, not only syntax. Whatever a generator's output must satisfy, the
generator is where to check it.

## Where it is held
- `make module` ends with "Generating and verifying": `go generate` in the new module
  (`internal/api.gen.go` from its spec fragment) and in `cmd/openenu` (the registry and the
  merged `backend/openapi.json`), then `go build ./...`, `go vet` and `go test` on the module.
  Any failure stops it with "the scaffolded module does not build or its test fails - this is
  a platform bug, not yours".
- The scaffolded `<name>_test.go` mounts the module on `platform.InMemory` and asserts that
  `GET /api/<kebab-name>` answers 200 with the module's name, so a template that compiles and
  answers wrongly fails too.
- `make selftest` scaffolds a module in a throwaway copy and asserts what the generator
  claims: the registry lists its permission, the merged spec carries its path, the Task Router
  row and the frontend layer exist, the layer sets no `srcDir`, the page is gated by the
  permission, a second scaffold and a StudlyCase name are refused, and the copy still passes
  `make check` with the module in it.

## Bash specifics worth remembering
In an unquoted heredoc: `$$` is the PID, `$(...)` and backticks execute, `${X}` expands. A
literal dollar needs `\$`. Quoting the delimiter (`<<'EOF'`) disables all of it - the right
default for templates, and only workable when nothing needs interpolating.

## Where else this applies
- `make init` and `make manager-module`: `selftest` runs both in a copy and asserts their
  result, `init` down to a byte-identical kernel tree and a tree that still builds.
- Any future scaffolder, codemod or migration generator.
- Same family as [[status-codes-alone-are-not-a-health-check]]: a tool that reports success
  because it only checked the thing that was easy to check.
