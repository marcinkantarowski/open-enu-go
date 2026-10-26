#!/usr/bin/env bash
# =============================================================================
#  Where a stage lives, derived - never configured per call.
# =============================================================================
#  Staging and production may share one server or have one each. Either way
#  they must never share anything else: not a directory, not a compose project,
#  not a database volume, not a systemd unit. Every name below carries the
#  stage for that reason - on one box, a name without it is a name two stacks
#  fight over, and the loser is whichever was provisioned first.
#
#      stage        stack               checkout                 compose files
#      staging      <slug>-staging      /opt/<slug>-staging      dev + staging override
#      production   <slug>-prod         /opt/<slug>-prod         prod
#
#  Also here: `on HOST cmd`, which runs a command on a host over ssh - or right
#  here when HOST is `local`, which is what lets `make promote` work from the
#  staging server itself when production is on the same box.
# =============================================================================

stage_normalize() {
  case "${1:-}" in
    staging|stg) echo staging ;;
    production|prod) echo production ;;
    *) return 1 ;;
  esac
}

stage_short() { [ "$1" = production ] && echo prod || echo staging; }

project_slug() {
  local root="$1" slug
  slug="$(sed -n 's/.*"slug"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$root/.project.json" | head -1)"
  echo "${slug:-open-enu-go}"
}

stage_stack() { echo "$1-$(stage_short "$2")"; }            # <slug> <stage>
stage_dir()   { echo "/opt/$(stage_stack "$1" "$2")"; }      # <slug> <stage>

stage_compose_files() {
  if [ "$1" = production ]; then
    echo "docker/compose.prod.yml"
  else
    echo "docker/compose.dev.yml:docker/compose.staging.yml"
  fi
}

# The SSH host name git uses for GitHub. Staging has its own alias because its
# deploy key can WRITE - it is where vibe-coded commits are pushed from - and
# production's must stay read-only. One ~/.ssh/config, two identities.
stage_git_host() { [ "$1" = production ] && echo github.com || echo github-staging; }

# on <host|local> <command>
on() {
  local host="$1"; shift
  if [ "$host" = local ]; then
    bash -c "$*"
  else
    ssh -o BatchMode=yes -o ConnectTimeout=15 "$host" "$@"
  fi
}
