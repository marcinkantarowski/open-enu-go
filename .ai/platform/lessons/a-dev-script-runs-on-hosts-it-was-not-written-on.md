---
tags: [tooling, shell, macos, portability]
date: 2026-10-25
---
# A dev script runs on hosts it was not written on

A script under `scripts/dev` or `scripts/lib` runs on the developer's machine - Linux, WSL2 or
macOS - and GNU and BSD tools disagree on flags that look universal. ShellCheck checks syntax
and quoting, not which implementation of `sed` is installed, and CI runs on Linux.

## The trap
GNU sed takes an optional backup suffix glued to `-i`. BSD sed, which macOS ships, requires
one (an empty one is allowed: `sed -i ''`), so given none it takes the expression as the
suffix and the file path as the script:

```bash
sed -i "s|^${key}=.*|${key}=${esc}|" "$file"   # $file is /Users/m.../.env
```

```
sed: 1: "/Users/m ...": invalid command code m
```

`/Users/` parses as an address and the `m` of the user name as a command. The error names the
path, not the flag, so it does not look like a portability problem - and it stops `make env`,
the second step of `make builddev`, on the first Mac it meets.

Nothing in `make ci` can catch it: the workflow runs on `ubuntu-latest`, and every Linux or
WSL2 host runs GNU sed. A platform that already knows about macOS (`os_kind` in
`scripts/lib/os.sh`, the Homebrew column in `scripts/lib/tools.sh`) claims support that no
step has executed there. A portability rule has to be a static check, not a test.

## The rule
**A script that will run on a developer's host is written for the lowest common
implementation, and the difference is wrapped once.**

`scripts/remote/` is exempt on purpose: it runs over ssh on the server, which is Debian or
Ubuntu (`make preflight` checks).

## Where it is held
- `scripts/lib/os.sh` has `sed_inplace`, which picks the GNU or the BSD form. `envgen.sh`,
  `init.sh` and `selftest.sh` call it; `module.sh` and `selftest.sh` also edit through a temp
  file and `mv`, which needs no `-i` at all.
- `make shell-check` (`scripts/dev/check-shell.sh`) fails on a bare in-place `sed` anywhere
  under `scripts/dev` or `scripts/lib`, before it runs ShellCheck.
- `AGENTS.md` (Never list).

## Where else this applies
- `grep -P`, `readlink -f`, `date -d`, `stat -c`, `\|` in a basic regex and `sed`'s `0,/re/`
  address are all GNU-only. "First match only" is an `awk` one-liner instead:
  `awk '!d && /re/ {d=1; next} {print}'`, as `selftest.sh` uses.
- BSD `wc -l` pads its count with spaces: compare it through `tr -d ' '`, or use `grep -c`.
- `timeout` is not on macOS by default; `check-ports.sh` falls back when it is absent.
- Anything inside a container is fine: the containers are Linux whatever the host is.
- `make platform-update` renders the project's base commit with that commit's own `init.sh`,
  so a portability fix has to reach back: when an old version cannot rename itself on this
  host, `platform-update.sh` renders it with the target's `init.sh` through `INIT_ROOT`, says
  so, and `make selftest` proves the fallback.
