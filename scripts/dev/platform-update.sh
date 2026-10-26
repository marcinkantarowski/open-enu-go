#!/usr/bin/env bash
# =============================================================================
#  make platform-update [FROM=<path or git url>] [REF=<ref>] [BASE=<commit>]
# =============================================================================
#  Brings a project up to a newer version of the platform it was made from.
#
#  A project is a renamed copy: `make init` rewrote the slug, the domain and the
#  application's Go module path - every import line of it. So the platform's own
#  diff does not apply to it - its context lines carry the platform's spelling
#  where the project has its own - and copying files across by hand means
#  redoing that rename by hand, differently each time.
#
#  FROM is the platform: a local checkout, or a git URL (cloned into a cache and
#  fetched on every run). REF is the commit wanted there, HEAD by default. Both
#  FROM and the commit reached are remembered in .project.json, so a later run
#  is just `make platform-update`.
#
#  This does it the way a merge does:
#
#    1. export the platform at the commit the project last had (BASE) and at
#       the one it wants (REF)
#    2. run each export through its OWN `init`, with this project's name, domain
#       and Go module path - so both are spelled the way the project is (a
#       version whose init cannot run on this host is renamed by the target's
#       init, and the run says so)
#    3. take the difference between those two and apply it to the project as a
#       three-way merge
#
#  What the project never touched changes silently. What both sides changed is
#  a conflict, marked in the file like any other, for a person to resolve. The
#  result is left STAGED and UNCOMMITTED: the update is one reviewable diff
#  (`git diff --cached`), and committing it is the user's act, not this
#  script's.
#
#  BASE is remembered in .project.json (`platform_ref`) after every run, so it
#  is only ever asked for once, on a project made before this script existed.
# =============================================================================
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
# shellcheck source=../lib/log.sh
. "$HERE/../lib/log.sh"

MANIFEST="$ROOT/.project.json"
manifest() { sed -n "s/.*\"$1\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" "$MANIFEST" | head -1; }

# Files a project regenerates from its own code. The platform's copy describes
# the platform's modules, so on a conflict the project's version is kept and
# the right answer is to regenerate, not to merge two machine-written files.
# The Go ones by pattern: every module's server is one.
GENERATED=(backend/openapi.json ui-kit/types/api.d.ts .ai/inventory.json)
generated() { # <path>
  local g
  for g in "${GENERATED[@]}"; do [ "$1" = "$g" ] && return 0; done
  case "$1" in backend/*.gen.go|backend/*_gen.go) return 0 ;; esac
  return 1
}

# ── preconditions ───────────────────────────────────────────────────────────
grep -q '"initialized"[[:space:]]*:[[:space:]]*true' "$MANIFEST" 2>/dev/null || die \
  "this checkout is the platform itself, not a project made from it" \
  "there is nothing to update it from - a project is what \`make init\` produces"

git -C "$ROOT" rev-parse --git-dir >/dev/null 2>&1 || die "this project is not a git repository" \
  "the update is applied as a merge and reviewed as a diff; both need git"

if ! git -C "$ROOT" diff --quiet || ! git -C "$ROOT" diff --cached --quiet; then
  die "there are uncommitted changes" \
    "commit or stash them first - the update must be the only thing in the diff you review"
fi

# No default path: a sibling checkout exists on one person's machine, and the
# answer is remembered after the first run anyway.
SOURCE="${FROM:-$(manifest platform_source)}"
[ -n "$SOURCE" ] || die "where is the platform?" \
  "usage: make platform-update FROM=<path to a checkout, or a git url> [REF=<ref>]" \
  "it is remembered in .project.json, so this is asked once"

case "$SOURCE" in
  *://*|*@*:*)
    # A URL: keep one clone per URL in a cache and fetch it, so an update does
    # not depend on a sibling directory that exists on one person's machine.
    cache="${XDG_CACHE_HOME:-$HOME/.cache}/enu-platform/$(printf '%s' "$SOURCE" | cksum | cut -d' ' -f1)"
    if [ -d "$cache" ]; then
      git -C "$cache" fetch -q --prune origin '+refs/heads/*:refs/heads/*' \
        || die "could not fetch the platform from $SOURCE"
    else
      mkdir -p "$(dirname "$cache")"
      git clone -q --bare "$SOURCE" "$cache" || die "could not clone the platform from $SOURCE"
    fi
    FROM="$cache"
    ;;
  *)
    [ -d "$SOURCE" ] || die "no platform checkout at $SOURCE" \
      "usage: make platform-update FROM=<path to a checkout, or a git url> [REF=<ref>]"
    FROM="$(cd "$SOURCE" && pwd)"
    git -C "$FROM" rev-parse --git-dir >/dev/null 2>&1 || die "$FROM is not a git repository"
    ;;
esac

REF="${REF:-HEAD}"
TARGET="$(git -C "$FROM" rev-parse --verify --quiet "${REF}^{commit}")" \
  || die "the platform has no commit '$REF'" "looked in: $SOURCE"

BASE="${BASE:-$(manifest platform_ref)}"
if [ -z "$BASE" ]; then
  # A guess is offered, never taken: starting from the wrong commit applies the
  # wrong diff, and that is worse than asking.
  first="$(git -C "$ROOT" log --reverse --format=%cI 2>/dev/null | head -1)"
  guess="$([ -n "$first" ] && git -C "$FROM" log -1 --format='%h  %s' --before="$first" "$TARGET" 2>/dev/null || true)"
  die "this project does not record which platform commit it was made from" \
    "say so once, and it is remembered: make platform-update BASE=<commit>" \
    "BASE is the platform commit this project already matches - the one it was" \
    "copied from, or the last one whose changes were brought across by hand." \
    ${guess:+"the newest platform commit older than this project's first: $guess"}
fi
BASE="$(git -C "$FROM" rev-parse --verify --quiet "${BASE}^{commit}")" \
  || die "the platform has no commit '${BASE}'" \
       "platform_ref in .project.json must name a commit in $SOURCE" \
       "override it once with: make platform-update BASE=<commit>"

SLUG="$(manifest slug)"; DOMAIN="$(manifest domain)"; GO_MODULE="$(manifest go_module)"
[ -n "$SLUG" ] && [ -n "$DOMAIN" ] && [ -n "$GO_MODULE" ] || die ".project.json has no slug, domain or go_module"

log_step "Platform update"
log_info "from     $SOURCE"
log_info "base     $(git -C "$FROM" log -1 --format='%h  %s' "$BASE")"
log_info "target   $(git -C "$FROM" log -1 --format='%h  %s' "$TARGET")"

record() {
  REF_VALUE="$1" SOURCE_VALUE="$SOURCE" python3 - "$MANIFEST" <<'PY'
import json, os, sys
path = sys.argv[1]
with open(path, encoding='utf-8') as fh:
    data = json.load(fh)
data['platform_ref'] = os.environ['REF_VALUE']
data['platform_source'] = os.environ['SOURCE_VALUE']
with open(path, 'w', encoding='utf-8') as fh:
    json.dump(data, fh, indent=2, ensure_ascii=False)
    fh.write('\n')
PY
  git -C "$ROOT" add "$MANIFEST"
}

if [ "$BASE" = "$TARGET" ]; then
  if [ "$(manifest platform_ref)" != "$TARGET" ] || [ "$(manifest platform_source)" != "$SOURCE" ]; then
    record "$TARGET"
    log_ok "already at that commit - recorded it in .project.json (staged)"
  else
    log_skip "already at that commit - nothing to do"
  fi
  exit 0
fi

# ── both versions, spelled the way this project is ──────────────────────────
WORK="$(mktemp -d)"; trap 'rm -rf "$WORK"' EXIT

export_at() { # <commit> <dir>
  rm -rf "$2"; mkdir -p "$2"
  git -C "$FROM" archive "$1" | tar -x -C "$2"
}

render() { # <commit> <dir> [<init.sh to fall back to>]
  export_at "$1" "$2"
  # Each version's own init: if init itself changed between the two, the
  # difference in what it renames is part of the update.
  if ! ( cd "$2" && NAME="$SLUG" DOMAIN="$DOMAIN" GO_MODULE="$GO_MODULE" bash scripts/dev/init.sh >/dev/null 2>&1 ); then
    # A version from before a portability fix cannot rename itself on the host
    # that needed the fix. The target's init can, pointed at a fresh export of
    # this version, with the target's own helpers.
    [ -n "${3:-}" ] || die "could not rename the platform at $(git -C "$FROM" rev-parse --short "$1") into '$SLUG'" \
      "its scripts/dev/init.sh failed - run it by hand in a scratch copy to see why"
    # An init that does not take INIT_ROOT would rename its own tree again and
    # leave this one as the platform spells it - a wrong diff with no error.
    grep -q 'INIT_ROOT' "$3" || die "could not rename the platform at $(git -C "$FROM" rev-parse --short "$1") into '$SLUG'" \
      "its own scripts/dev/init.sh failed here, and the target's init predates INIT_ROOT - update to a newer target"
    export_at "$1" "$2"
    INIT_ROOT="$2" NAME="$SLUG" DOMAIN="$DOMAIN" GO_MODULE="$GO_MODULE" bash "$3" >/dev/null 2>&1 \
      || die "could not rename the platform at $(git -C "$FROM" rev-parse --short "$1") into '$SLUG'" \
           "neither its own scripts/dev/init.sh nor the target's could run here - run one by hand in a scratch copy to see why"
    log_warn "the platform at $(git -C "$FROM" rev-parse --short "$1") cannot run its own init on this host - renamed with the target's init instead"
  fi
  # The manifest is the project's identity, not something the platform updates.
  rm -f "$2/.project.json"
}

render "$TARGET" "$WORK/target"
render "$BASE"   "$WORK/base" "$WORK/target/scripts/dev/init.sh"

# A throwaway repository holding exactly two commits, so git can do the merge:
# base, and target on top of it.
G=(git -c user.name=platform-update -c user.email=platform-update@localhost -c core.autocrlf=false -c commit.gpgsign=false)
"${G[@]}" init -q "$WORK/repo"
"${G[@]}" -C "$WORK/repo" --work-tree="$WORK/base" add -A -f . >/dev/null
"${G[@]}" -C "$WORK/repo" commit -q --allow-empty -m "platform at base"
"${G[@]}" -C "$WORK/repo" --work-tree="$WORK/target" add -A -f . >/dev/null
"${G[@]}" -C "$WORK/repo" commit -q --allow-empty -m "platform at target"
"${G[@]}" -C "$WORK/repo" branch -q -f platform-target

if [ -z "$("${G[@]}" -C "$WORK/repo" diff --name-only HEAD~1 HEAD)" ]; then
  record "$TARGET"
  log_ok "nothing in that range reaches a project - recorded the new commit (staged)"
  exit 0
fi

# ── apply it as a merge ─────────────────────────────────────────────────────
git -C "$ROOT" fetch -q --no-tags "$WORK/repo" platform-target

conflicted=0
"${G[@]}" -C "$ROOT" cherry-pick --no-commit --allow-empty FETCH_HEAD >/dev/null 2>"$WORK/merge.err" || conflicted=1

unmerged() { git -C "$ROOT" diff --name-only --diff-filter=U; }

if [ "$conflicted" -eq 1 ] && [ -z "$(unmerged)" ]; then
  # Refused outright rather than conflicted - an untracked file in the way, for
  # instance. Nothing was applied, so say why and leave the tree as it was.
  sed 's/^/    /' "$WORK/merge.err" >&2
  die "the update could not be applied" "the message above is git's; nothing was changed"
fi

regenerate=0
while IFS= read -r f; do
  generated "$f" || continue
  git -C "$ROOT" checkout --ours -- "$f" 2>/dev/null && git -C "$ROOT" add "$f"
  regenerate=1
done < <(unmerged)
while IFS= read -r f; do
  generated "$f" && regenerate=1
done < <(git -C "$ROOT" diff --cached --name-only)

record "$TARGET"

changed="$(git -C "$ROOT" diff --cached --name-only | wc -l | tr -d ' ')"
left="$(unmerged)"

log_ok "$changed file(s) updated and staged - nothing is committed"
[ "$regenerate" -eq 1 ] && log_warn "generated files are involved - regenerate them: make generate types inventory"

if [ -n "$left" ]; then
  log_fail "$(wc -l <<<"$left" | tr -d ' ') file(s) changed on both sides and need a decision" \
    "each has conflict markers; edit it, then: git add <file>" || true
  sed 's/^/      /' <<<"$left" >&2
  log_info "to abandon the update instead: git reset --hard"
  exit 1
fi

log_step "Next"
log_info "1. read it:        git diff --cached"
log_info "2. prove it:       make check   (then make ci)"
log_info "3. keep it:        commit - or abandon it with: git reset --hard"
