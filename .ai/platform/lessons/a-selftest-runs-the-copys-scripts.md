# A selftest runs the copy's scripts, not the real tree's

Every dev script here resolves its own `ROOT` from where the script file lives, not from the
working directory, precisely so that it works from any directory. A selftest that `cd`s into a
throwaway copy and then runs the real tree's script exercises the real tree.

## The trap
`selftest.sh` makes a throwaway copy of the tree, `cd`s into it, and exercises `make init`
there. Written the obvious way:

```bash
cd "$WORK"
NAME=acme-crm DOMAIN=crm.acme.test bash "$ROOT/scripts/dev/init.sh"
```

it renames the **real** repository: `init.sh` takes its `ROOT` from its own path, so the real
`.project.json`, `backend/go.mod` and `Makefile` say `acme-crm`, and `make check` keeps
passing, because a renamed tree is a valid tree. The assertions then fail in a misleading way:
the copy's manifest is untouched (the test looks there) while the real one has changed
(nothing looks there).

## The rule
A selftest runs **the copy's own scripts** (`$WORK/scripts/...`), never `$ROOT`'s. And it
guards the one thing that must never happen: it snapshots the real manifest before it starts
and fails loudly at exit if that snapshot differs.

`INIT_ROOT` exists for the one legitimate case of running a script against another tree:
`platform-update.sh` renders an old platform commit with the target's `init.sh`. A selftest is
not that case, because the whole point is to exercise the script as it ships.

## Where it is held
`scripts/dev/selftest.sh` sets `INIT="$WORK/scripts/dev/init.sh"` and runs every script from
`$WORK`. Its `EXIT` trap compares the real `.project.json` with the snapshot and fails with
"selftest modified the REAL .project.json - a script ran from $ROOT instead of the copy",
naming the `git checkout` that undoes it.

## Why the guard, not just the fix
The fix is one path. The guard is what makes the next person's version of the same mistake
show up as "selftest modified the REAL .project.json" instead of as a silently renamed
repository. A check that only passes when nothing went wrong has to also say so when something
did.

## Where else this applies
- `module.sh` restarts the frontend container only when that container mounts this checkout:
  `make selftest` runs the generator in a copy that can share the stack's name, and restarting
  the real dev server from there would be a side effect nobody asked for.
- Any script that edits "its" tree: it edits the tree it lives in, whatever the caller's
  working directory says.
