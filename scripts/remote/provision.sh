#!/usr/bin/env bash
# =============================================================================
#  Bare server → every subdomain live, in one command (.ai/platform/docs/deployment.md).
# =============================================================================
#  Runs from YOUR machine and drives the server over SSH. Every step is
#  idempotent: re-running against a live server is the normal case, and the
#  property that makes that safe is that secrets and data are read, never
#  regenerated.
#
#  Usage: provision.sh HOST DOMAIN STAGE [REPO] [REF]
#         STAGE = staging | production
#
#  Staging and production may share a server or have one each. Each gets its
#  own checkout, compose project, database, .env and systemd units
#  (scripts/lib/stage.sh); the only thing they share on one box is the edge.
#
#    production  immutable images, off-box backups
#    staging     the DEV stack - hot reload, Mailpit - so you can vibe code on
#                it; reachable only from STAGING_ALLOW_FROM, and its git key
#                can push, because commits made there are what `make promote`
#                ships
#
#  Optional environment: EMAIL (LETSENCRYPT_EMAIL), ALLOW (STAGING_ALLOW_FROM;
#  defaults to the address you are connecting from), SSH_ALLOW_FROM (who may
#  reach port 22; empty = anywhere, with a warning - kept in the stage's .env).
#
#  HOST names the deploy user: deploy@203.0.113.42. On a bare box it is created
#  first, from root@, with passwordless sudo and the docker group, and a second
#  login as it is proved before anything else happens (deploy-user.sh).
#  Production refuses to run as root@ - the units would run as root too.
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"; . "$HERE/../lib/stage.sh"

HOST="${1:-}"; DOMAIN="${2:-}"; REPO="${4:-}"; REF="${5:-main}"
STAGE="$(stage_normalize "${3:-staging}")" || die "STAGE must be staging or production, got: ${3:-}"
SYNC="${SYNC:-git}"
SLUG="$(project_slug "$ROOT")"
STACK="$(stage_stack "$SLUG" "$STAGE")"
APP_DIR="$(stage_dir "$SLUG" "$STAGE")"
GIT_HOST="$(stage_git_host "$STAGE")"
OTHER_DIR="$(stage_dir "$SLUG" "$([ "$STAGE" = production ] && echo staging || echo production)")"

[ -n "$HOST" ] && [ -n "$DOMAIN" ] || die "usage: provision.sh HOST DOMAIN STAGE [REPO] [REF]"

REMOTE() { ssh -o BatchMode=yes -o ConnectTimeout=15 "$HOST" "$@"; }
DEPLOY_USER="${HOST%@*}"
[ "$DEPLOY_USER" = "$HOST" ] && DEPLOY_USER=root
SERVER="${HOST#*@}"

if [ "$DEPLOY_USER" = root ]; then
  if [ "$STAGE" = production ]; then
    die "production does not run as root - HOST is $HOST" \
      "name a deploy user and this creates it from root@: make buildprod HOST=deploy@$SERVER DOMAIN=$DOMAIN REPO=…" \
      "the stack, the backups and every unit run as that user; sshd is hardened only once a login as it is proved"
  fi
  log_warn "provisioning $STAGE as root: no deploy user, sshd is not hardened, the units run as root"
fi

# ── 0a. the deploy user, before anything that logs in as it ─────────────────
# Preflight, the deploy key and every step below log in as HOST; on a bare box
# that user does not exist yet. Creating it is the one change made before the
# read-only preflight, and it touches nothing but the user: no sshd, no ports.
if [ "$DEPLOY_USER" != root ]; then
  "$HERE/deploy-user.sh" "$HOST" || die "the deploy user is not ready - nothing else was touched"
fi

# ── 0. preflight ────────────────────────────────────────────────────────────
if [ "${SKIP_PREFLIGHT:-0}" = "1" ]; then
  # Documented, and a footgun: it exists for the case where you know DNS is
  # still propagating and want the stack up anyway. It is not a shortcut.
  log_warn "SKIP_PREFLIGHT=1 - provisioning a server without checking DNS, ports or repo access"
else
  GIT_HOST="$GIT_HOST" "$HERE/preflight.sh" "$HOST" "$DOMAIN" "$REPO" "$REF" || die "preflight failed - nothing was touched"
fi

log_step "Provisioning $SERVER for $DOMAIN ($STAGE → $STACK in $APP_DIR)"

# ── 1. Docker and git, if missing ───────────────────────────────────────────
if REMOTE "command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1"; then
  log_skip "docker and compose already installed"
else
  log_step "Installing Docker"
  REMOTE "sudo -n true 2>/dev/null || [ \$(id -u) -eq 0 ]" \
    || die "installing Docker needs root on the server" \
         "run this once as root@, or give $DEPLOY_USER passwordless sudo"
  REMOTE "set -e
    SUDO=\$([ \$(id -u) -eq 0 ] && echo '' || echo sudo)
    export DEBIAN_FRONTEND=noninteractive
    \$SUDO apt-get update -qq
    \$SUDO apt-get install -y -qq ca-certificates curl git make ufw
    \$SUDO install -m 0755 -d /etc/apt/keyrings
    curl -fsSL https://download.docker.com/linux/\$(. /etc/os-release && echo \$ID)/gpg \
      | \$SUDO tee /etc/apt/keyrings/docker.asc >/dev/null
    \$SUDO chmod a+r /etc/apt/keyrings/docker.asc
    echo \"deb [arch=\$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/\$(. /etc/os-release && echo \$ID) \$(. /etc/os-release && echo \$VERSION_CODENAME) stable\" \
      | \$SUDO tee /etc/apt/sources.list.d/docker.list >/dev/null
    \$SUDO apt-get update -qq
    \$SUDO apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
  " || die "Docker installation failed"
  log_ok "docker installed"
fi

# The nightly backup encrypts with age and leaves with rclone. Without them it
# keeps the dump on the same disk and says so in a log nobody reads - so they are
# installed here, not left for somebody to remember. Separate from Docker: a
# server image with Docker already on it has neither.
if REMOTE "command -v age >/dev/null 2>&1 && command -v rclone >/dev/null 2>&1"; then
  log_skip "age and rclone already installed"
else
  REMOTE "set -e
    SUDO=\$([ \$(id -u) -eq 0 ] && echo '' || echo sudo)
    export DEBIAN_FRONTEND=noninteractive
    \$SUDO apt-get update -qq
    \$SUDO apt-get install -y -qq age rclone
  " || die "installing age and rclone failed" \
         "they need root on the server, like Docker: run once as root@, or give $DEPLOY_USER passwordless sudo"
  log_ok "age and rclone installed"
fi

# ── 2. the deploy user is in the docker group ───────────────────────────────
# deploy-user.sh (step 0a) added it; on a box where Docker came later, or a
# user made by hand, this is where it is made true. Root-equivalent, and said
# so in deployment.md: every stack command runs as this user.
if [ "$DEPLOY_USER" != root ]; then
  if REMOTE "id -nG | tr ' ' '\\n' | grep -qx docker"; then
    log_skip "$DEPLOY_USER is in the docker group"
  elif REMOTE "sudo -n usermod -aG docker $DEPLOY_USER"; then
    log_ok "$DEPLOY_USER added to the docker group (takes effect on the next login)"
  else
    die "could not add $DEPLOY_USER to the docker group" "by hand: sudo usermod -aG docker $DEPLOY_USER"
  fi
fi

# ── 3. hardening, gated on a PROVEN second login ────────────────────────────
if [ "$DEPLOY_USER" != "root" ]; then
  log_step "Verifying the deploy login before hardening"
  if ! REMOTE true 2>/dev/null; then
    die "cannot log in as $HOST" \
      "sshd_config was NOT modified - the box is exactly as you found it" \
      "fix the key for $DEPLOY_USER and re-run"
  fi
  REMOTE "sudo -n true" 2>/dev/null || die "$DEPLOY_USER logs in but has no passwordless sudo" \
    "sshd_config was NOT modified" "re-run: bash scripts/remote/deploy-user.sh $HOST"
  log_ok "second login as $DEPLOY_USER works, with sudo"

  # The EFFECTIVE configuration (sshd -T), not a grep of one file: Ubuntu's
  # cloud images ship sshd_config.d/50-cloud-init.conf with password auth ON,
  # and the first value sshd reads wins - so the setting goes in a drop-in
  # named to be read first, and is then checked the way sshd sees it.
  if REMOTE "sudo -n sshd -T 2>/dev/null | grep -qx 'passwordauthentication no'"; then
    log_skip "sshd already hardened"
  else
    log_step "Disabling password auth and root login"
    REMOTE "set -e
      sudo -n cp /etc/ssh/sshd_config /etc/ssh/sshd_config.bak.\$(date +%s)
      sudo -n sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication no/' /etc/ssh/sshd_config
      sudo -n sed -i 's/^#\?PermitRootLogin.*/PermitRootLogin prohibit-password/' /etc/ssh/sshd_config
      if [ -d /etc/ssh/sshd_config.d ]; then
        printf 'PasswordAuthentication no\nKbdInteractiveAuthentication no\nPermitRootLogin prohibit-password\n' \
          | sudo -n tee /etc/ssh/sshd_config.d/00-enu-hardening.conf >/dev/null
      fi
      sudo -n sshd -t
      sudo -n systemctl reload ssh 2>/dev/null || sudo -n systemctl reload sshd
      sudo -n sshd -T | grep -qx 'passwordauthentication no'
    " && log_ok "password auth off, root login key-only (backup kept)" \
      || log_warn "could not harden sshd - the stack still works; do it by hand"
    # And the door is still open: a NEW login after the reload.
    REMOTE true 2>/dev/null || die "a new login as $HOST fails after the sshd change" \
      "an existing session is not affected - restore from /etc/ssh/sshd_config.bak.* via the provider's console"
  fi
fi

# ── 4. host configuration, step by step ─────────────────────────────────────
# Each step reports for itself. "firewall, log rotation, swap ✓" printed after
# a block that may have failed half-way is how a box runs for a year without
# the firewall anybody thinks it has.
log_step "Host configuration"
HOST_FAILED=()
# host_step <label> <script> [args...] - the script runs under bash on the
# server with SUDO set; its stdout is shown indented, its exit decides.
host_step() {
  local label="$1" script="$2" out; shift 2
  # Arguments quoted for the remote shell: ssh joins them into one string, and
  # an empty one would vanish and shift the rest.
  if out="$(REMOTE bash -s -- "$(printf '%q ' "$@")" <<<"set -e; SUDO=\$([ \$(id -u) -eq 0 ] && echo '' || echo 'sudo -n'); export DEBIAN_FRONTEND=noninteractive; $script" 2>&1)"; then
    log_ok "$label"
    [ -z "$out" ] || sed 's/^/      /' <<<"$out"
  else
    log_warn "$label - did not apply"
    [ -z "$out" ] || tail -5 <<<"$out" | sed 's/^/      /' >&2
    HOST_FAILED+=("$label")
  fi
}

# Who may reach port 22. From the environment, else what this stage's .env
# kept from the last run, else anywhere.
CLIENT_IP="$(REMOTE 'echo ${SSH_CLIENT%% *}' 2>/dev/null || true)"
SSH_ALLOW="${SSH_ALLOW_FROM:-$(REMOTE "sed -n 's/^SSH_ALLOW_FROM=//p' $APP_DIR/.env 2>/dev/null | head -1" 2>/dev/null || true)}"
SSH_ALLOW="${SSH_ALLOW// /}"
if [ -n "$SSH_ALLOW" ]; then
  # Never close the door you are standing in: the address this run comes from
  # must be inside the list, or the list is not applied.
  if [ -z "$CLIENT_IP" ] || ! python3 -c 'import ipaddress,sys; ip=ipaddress.ip_address(sys.argv[1]); sys.exit(0 if any(ip in ipaddress.ip_network(n, strict=False) for n in sys.argv[2].split(",") if n) else 1)' "$CLIENT_IP" "$SSH_ALLOW" 2>/dev/null; then
    die "SSH_ALLOW_FROM ($SSH_ALLOW) does not include the address this run comes from (${CLIENT_IP:-unknown})" \
      "applying it would lock this machine out of the server - nothing was changed" \
      "add ${CLIENT_IP:-your address} to SSH_ALLOW_FROM, or leave it empty"
  fi
fi

host_step "packages: ufw, fail2ban, unattended-upgrades" '
  if ! dpkg -s ufw fail2ban unattended-upgrades >/dev/null 2>&1; then
    $SUDO apt-get update -qq
    $SUDO apt-get install -y -qq ufw fail2ban unattended-upgrades >/dev/null
  fi'

host_step "firewall: 22 from ${SSH_ALLOW:-anywhere}, 80, 443" '
  SRC="$1"
  if [ -z "$SRC" ]; then
    $SUDO ufw allow 22/tcp >/dev/null
    stale="$($SUDO ufw status numbered | sed -n "s/^\[ *\([0-9]*\)\].*# enu-ssh\$/\1/p" | sort -rn)"
  else
    for s in ${SRC//,/ }; do $SUDO ufw allow from "$s" to any port 22 proto tcp comment enu-ssh >/dev/null; done
    # Rules for sources no longer on the list, newest number first so the
    # numbering of the rest does not move under the loop.
    stale="$($SUDO ufw status numbered | sed -n "s/^\[ *\([0-9]*\)\][^A]*ALLOW IN *\([^ ]*\).*# enu-ssh\$/\1 \2/p" \
             | while read -r n from; do case ",$SRC," in *",$from,"*) ;; *) echo "$n" ;; esac; done | sort -rn)"
  fi
  for n in $stale; do $SUDO ufw --force delete "$n" >/dev/null; done
  $SUDO ufw allow 80/tcp  >/dev/null
  $SUDO ufw allow 443/tcp >/dev/null
  [ -z "$SRC" ] || $SUDO ufw delete allow 22/tcp >/dev/null 2>&1 || true
  $SUDO ufw --force enable >/dev/null
  $SUDO ufw status | grep -q "Status: active"' "$SSH_ALLOW"
if [ -z "$SSH_ALLOW" ]; then
  log_warn "port 22 is open to anywhere - SSH_ALLOW_FROM=<your ip or cidr> narrows it (fail2ban guards it meanwhile)"
elif ! REMOTE true 2>/dev/null; then
  die "the firewall change locked out a new login from here" \
    "from the provider's console: sudo ufw allow 22/tcp" \
    "then fix SSH_ALLOW_FROM and re-run"
else
  log_ok "a new login still works through the allowlist"
fi

host_step "fail2ban: sshd jail (5 tries in 10 minutes, banned for an hour)" '
  IGNORE="127.0.0.1/8 ::1 ${1//,/ } $2"
  printf "[sshd]\nenabled = true\nbackend = systemd\nmaxretry = 5\nfindtime = 10m\nbantime = 1h\nignoreip = %s\n" "$IGNORE" \
    | $SUDO tee /etc/fail2ban/jail.d/sshd.local >/dev/null
  $SUDO systemctl enable fail2ban >/dev/null 2>&1
  $SUDO systemctl restart fail2ban
  for _ in 1 2 3 4 5 6 7 8 9 10; do $SUDO fail2ban-client ping >/dev/null 2>&1 && break; sleep 1; done
  $SUDO fail2ban-client status sshd >/dev/null' "$SSH_ALLOW" "$CLIENT_IP"

# Security updates only, applied daily, never a reboot: a point release or
# Docker's own packages moving under a running stack is a deploy nobody chose.
host_step "unattended-upgrades: security only" '
  printf "APT::Periodic::Update-Package-Lists \"1\";\nAPT::Periodic::Unattended-Upgrade \"1\";\n" \
    | $SUDO tee /etc/apt/apt.conf.d/20auto-upgrades >/dev/null
  printf "%s\n" "#clear Unattended-Upgrade::Origins-Pattern;" \
    "Unattended-Upgrade::Origins-Pattern {" \
    "  \"origin=Debian,codename=\${distro_codename}-security,label=Debian-Security\";" \
    "  \"origin=Ubuntu,archive=\${distro_codename}-security\";" \
    "};" \
    "Unattended-Upgrade::Automatic-Reboot \"false\";" \
    | $SUDO tee /etc/apt/apt.conf.d/52enu-security-only >/dev/null
  apt-config dump | grep -q "Origins-Pattern:: \"origin=.*security"
  $SUDO systemctl enable --now unattended-upgrades >/dev/null 2>&1'

# TLS, tokens and the backup timer all read the clock. Checked, and asked to
# sync when it is not; a clock that is not synchronised YET is a warning.
host_step "time: NTP synchronised" '
  [ "$(timedatectl show -p NTPSynchronized --value 2>/dev/null)" = yes ] && exit 0
  $SUDO timedatectl set-ntp true 2>/dev/null || true
  echo "not synchronised yet - NTP switched on; check: timedatectl status"
  exit 1'

# Docker's default is unbounded. One chatty container fills the disk the
# database is on, and Postgres stops before anything says why. Restarted only
# when the file changed: a restart of Docker restarts every running container.
host_step "docker: log rotation (10m x 3)" '
  want="{\"log-driver\":\"json-file\",\"log-opts\":{\"max-size\":\"10m\",\"max-file\":\"3\"}}"
  if [ "$(cat /etc/docker/daemon.json 2>/dev/null)" != "$want" ]; then
    echo "$want" | $SUDO tee /etc/docker/daemon.json >/dev/null
    $SUDO systemctl restart docker
    echo "daemon.json written, docker restarted"
  fi'

# Under 2GB, a Nuxt build is the thing that gets OOM-killed. The Go build
# is light; the three Nuxt images are not.
host_step "swap: 2G under 2GB of RAM" '
  if [ "$(free -m | awk "/^Mem:/{print \$2}")" -lt 1900 ] && [ ! -f /swapfile ]; then
    $SUDO fallocate -l 2G /swapfile && $SUDO chmod 600 /swapfile && $SUDO mkswap /swapfile >/dev/null
    $SUDO swapon /swapfile
    grep -q "^/swapfile" /etc/fstab || echo "/swapfile none swap sw 0 0" | $SUDO tee -a /etc/fstab >/dev/null
    echo "added 2G swap"
  fi'

if [ ${#HOST_FAILED[@]} -gt 0 ]; then
  log_warn "host configuration incomplete: ${HOST_FAILED[*]}"
  log_info "the stack can still come up; fix these by hand or re-run - each step is idempotent"
fi

# ── 5. the code ─────────────────────────────────────────────────────────────
log_step "Syncing code to $APP_DIR"
REMOTE "sudo -n mkdir -p $APP_DIR /opt/enu-edge && sudo -n chown $DEPLOY_USER:$DEPLOY_USER $APP_DIR /opt/enu-edge" 2>/dev/null \
  || REMOTE "mkdir -p $APP_DIR /opt/enu-edge"

if REMOTE "test -d $OTHER_DIR" 2>/dev/null; then
  RAM_MB="$(REMOTE "free -m | awk '/^Mem:/{print \$2}'" 2>/dev/null || echo 0)"
  if [ "${RAM_MB:-0}" -lt 3800 ]; then
    log_warn "staging and production on one server with ${RAM_MB}MB RAM - the dev stack alone runs three Nuxt dev servers; 4GB is the floor"
  fi
fi

if [ "$SYNC" = rsync ]; then
  # Day one, before the project has a git remote. Documented, not hidden: what
  # lands on the server is your working tree, including anything uncommitted -
  # and with no git history there, `make promote` has nothing to promote.
  log_warn "SYNC=rsync - pushing the local working tree, not a commit; make promote will not work"
  command -v rsync >/dev/null 2>&1 || die "rsync is not installed"
  rsync -az --delete \
    --exclude '.git' --exclude 'node_modules' --exclude 'backend/bin' \
    --exclude '.nuxt' --exclude '.output' --exclude '.out' \
    --exclude 'docker/certs' --exclude 'docker/api/project.env' \
    --exclude '.env' --exclude '.env.*' \
    "$ROOT/" "$HOST:$APP_DIR/" || die "rsync failed"
  log_ok "working tree pushed"
else
  [ -n "$REPO" ] || die "REPO is required unless SYNC=rsync" \
    "example: make build$( [ "$STAGE" = production ] && echo prod || echo staging ) HOST=$HOST DOMAIN=$DOMAIN REPO=acme/$SLUG"
  if REMOTE "test -d $APP_DIR/.git"; then
    if [ "$STAGE" = staging ]; then
      # Staging is where work happens. Re-provisioning must not reset it.
      if [ -n "$(REMOTE "cd $APP_DIR && git status --porcelain")" ]; then
        log_warn "staging has uncommitted changes - leaving its checkout exactly as it is"
      else
        REMOTE "cd $APP_DIR && git fetch --quiet origin && git merge --quiet --ff-only origin/\$(git symbolic-ref --short HEAD)" \
          && log_ok "staging fast-forwarded" \
          || log_warn "staging has commits origin lacks - not touched; make promote pushes them"
      fi
    else
      REMOTE "cd $APP_DIR && git fetch --quiet origin && { git -c advice.detachedHead=false checkout --quiet --detach origin/$REF 2>/dev/null \
                || git -c advice.detachedHead=false checkout --quiet --detach $REF; }" \
        || die "could not move $APP_DIR to $REF"
      log_ok "$REPO@$REF at $APP_DIR"
    fi
  else
    REMOTE "git clone --quiet git@$GIT_HOST:$REPO.git $APP_DIR && cd $APP_DIR && git checkout --quiet $REF" \
      || die "could not clone $REPO@$REF" "run: make deploy-key HOST=$HOST REPO=$REPO STAGE=$STAGE"
    log_ok "$REPO@$REF cloned to $APP_DIR"
  fi

  if [ "$STAGE" = staging ]; then
    # Commits made on staging need an author. Yours, taken from this machine.
    name="$(git config user.name 2>/dev/null || true)"; email="$(git config user.email 2>/dev/null || true)"
    [ -n "$name" ] && REMOTE "cd $APP_DIR && git config user.name >/dev/null || git config user.name '$name'"
    [ -n "$email" ] && REMOTE "cd $APP_DIR && git config user.email >/dev/null || git config user.email '$email'"
  fi
fi

# ── 6. environment: generated once, preserved forever after ─────────────────
# This is the property that makes the command safe to re-run against a live
# server. Regenerating APP_SECRET invalidates every session; regenerating
# APP_ENCRYPTION_KEY makes every encrypted column unreadable, permanently.
log_step "Environment"

REMOTE "set -e
  cd $APP_DIR
  [ -f .env ] || { cp .env.example .env && chmod 600 .env && echo created; }
  ./scripts/lib/envgen.sh generate >/dev/null     # adds keys a newer .env.example declares
  set_env() { grep -q \"^\$1=\" .env && sed -i \"s|^\$1=.*|\$1=\$2|\" .env || echo \"\$1=\$2\" >> .env; }
  set_env DOMAIN       '$DOMAIN'
  set_env APP_STAGE    '$STAGE'
  set_env STACK        '$STACK'
  set_env COMPOSE_FILE '$(stage_compose_files "$STAGE")'
  set_env HOST_UID     \$(id -u)
  set_env HOST_GID     \$(id -g)
  [ -z '${EMAIL:-}' ] || set_env LETSENCRYPT_EMAIL '${EMAIL:-}'
  [ -z '$SSH_ALLOW' ] || set_env SSH_ALLOW_FROM '$SSH_ALLOW'
  if [ '$STAGE' = staging ]; then
    if [ -n '${ALLOW:-}' ]; then set_env STAGING_ALLOW_FROM '${ALLOW:-}'
    elif [ -z \"\$(sed -n 's/^STAGING_ALLOW_FROM=//p' .env)\" ] && [ -n '$CLIENT_IP' ]; then set_env STAGING_ALLOW_FROM '$CLIENT_IP'
    fi
  fi
  ./scripts/lib/envgen.sh generate >/dev/null
" || die "could not prepare the environment on $HOST"

[ -n "$(REMOTE "sed -n 's/^LETSENCRYPT_EMAIL=//p' $APP_DIR/.env")" ] \
  || die "LETSENCRYPT_EMAIL is not set for $STACK" \
       "without it no certificate is ever issued - re-run with EMAIL=you@example.com"
log_ok ".env present, per-app files derived (existing values preserved)"
[ "$STAGE" = staging ] && log_info "staging answers only to: $(REMOTE "sed -n 's/^STAGING_ALLOW_FROM=//p' $APP_DIR/.env")"

# Production refuses to start with Mailpit as its mail provider (compose.prod.yml
# requires MAILER_DSN). Said here, before a forty-minute build fails on it.
if [ "$STAGE" = production ]; then
  dsn="$(REMOTE "sed -n 's/^MAILER_DSN=//p' $APP_DIR/.env" 2>/dev/null || true)"
  case "$dsn" in
    ""|*mailpit*)
      die "MAILER_DSN is '${dsn:-empty}' in $APP_DIR/.env - production needs a real mail provider" \
        "set it on the server (smtp://user:pass@host:587) and re-run" \
        "silently dropping verification mail in production is worse than failing here" ;;
  esac
  log_ok "mail goes through a real provider"
fi

# ── 7. release: build, back up, migrate, start, health ─────────────────────
# The same script `make deploy*` and `make promote` run, so a first release
# and the hundredth cannot differ.
log_step "Releasing (a few minutes on the first run)"
REMOTE "cd $APP_DIR && bash scripts/remote/release.sh" \
  || die "the release failed on $HOST" "inspect: ssh $HOST 'cd $APP_DIR && docker compose logs --tail=80'"

# ── 8. from the outside: DNS, certificate and routing, end to end ───────────
log_step "Waiting for https://api.$DOMAIN/health/deep from here"
# From outside, /health/deep answers only the token the server's .env holds.
HEALTH_TOKEN="$(REMOTE "sed -n 's/^HEALTH_TOKEN=//p' $APP_DIR/.env 2>/dev/null | head -1" 2>/dev/null || true)"
healthy=0
for _ in $(seq 1 40); do
  if curl -fsS --max-time 5 -H "X-Health-Token: $HEALTH_TOKEN" "https://api.$DOMAIN/health/deep" 2>/dev/null | grep -q '"status":"ok"'; then
    healthy=1; break
  fi
  sleep 5
done
if [ "$healthy" -ne 1 ]; then
  [ "$STAGE" = staging ] && log_info "staging answers only to STAGING_ALLOW_FROM - is this machine's address on it?"
  die "https://api.$DOMAIN is not healthy from here within 200s" \
    "it is healthy on the server, so look at DNS, the certificate, or the allowlist:" \
    "  ssh $HOST 'docker logs --tail=80 enu-edge'"
fi
log_ok "deep health check green from outside"

# ── 9. survive a reboot, and back up nightly ────────────────────────────────
"$HERE/install-units.sh" "$HOST" "$STACK" "$APP_DIR" "$STAGE" "$DEPLOY_USER" || log_warn "systemd units not installed - the stack will not return after a reboot"

# ── 10. the backup gate - production only ──────────────────────────────────
BACKUP_REMOTE="$(REMOTE "grep '^BACKUP_REMOTE=' $APP_DIR/.env 2>/dev/null | cut -d= -f2-" || true)"
AGE_PUB="$(REMOTE "grep '^BACKUP_AGE_PUBKEY=' $APP_DIR/.env 2>/dev/null | cut -d= -f2-" || true)"

echo
"$HERE/urls.sh" "$DOMAIN" 2>/dev/null || true
[ -n "$AGE_PUB" ] && { echo; log_info "Backup public key (store the PRIVATE half off this server):"; log_info "  $AGE_PUB"; }

if [ -z "$BACKUP_REMOTE" ]; then
  if [ "$STAGE" = production ]; then
    # The stack is up. The command is not done - and saying "done" here is how a
    # production system runs for a year with backups nobody configured.
    echo
    log_fail "BACKUP_REMOTE is not set - this is a production server with no off-box backup" \
      "the stack IS running; this command is refusing to call that finished" \
      "set it on the server and re-run:" \
      "  ssh $HOST 'cd $APP_DIR && nano .env'   # BACKUP_REMOTE=s3:bucket/path" \
      "  make buildprod HOST=$HOST DOMAIN=$DOMAIN REPO=$REPO" \
      "a pg_dump on the same disk as the database is a hope, not a backup"
    exit 1
  fi
  log_warn "BACKUP_REMOTE is not set - local dumps only, and the nightly timer will warn until it is"
elif [ "$STAGE" = production ]; then
  # Set is not working. A remote nobody can log in to, or a dump that left in the
  # clear, looks exactly like a backup until the night it is needed - so one is
  # taken now and looked for where it is supposed to be.
  if [ -z "$AGE_PUB" ]; then
    log_fail "BACKUP_AGE_PUBKEY is not set - the dump would leave this server unencrypted" \
      "generate the pair on YOUR machine:  age-keygen -o ~/.config/age/<project>.txt" \
      "put the public half (age1...) in .env on the server; the private half never goes there"
    exit 1
  fi
  log_step "Proving the off-box backup"
  if ! REMOTE "cd $APP_DIR && bash scripts/remote/backup.sh manual" >/dev/null 2>&1; then
    log_fail "the backup did not reach $BACKUP_REMOTE" \
      "see why:  ssh $HOST 'cd $APP_DIR && bash scripts/remote/backup.sh manual'" \
      "the rclone remote belongs to the deploy user:  ssh $HOST rclone config"
    exit 1
  fi
  FOUND="$(REMOTE "rclone lsf '$BACKUP_REMOTE/' --include '*-manual.dump.age' 2>/dev/null | wc -l" || echo 0)"
  if [ "${FOUND:-0}" -eq 0 ]; then
    log_fail "the backup reported success but nothing is listed at $BACKUP_REMOTE" \
      "look by hand:  ssh $HOST \"rclone lsf '$BACKUP_REMOTE/'\""
    exit 1
  fi
  log_ok "an encrypted backup is at $BACKUP_REMOTE"
fi

echo
log_ok "$DOMAIN is live ($STAGE)"
if [ "$STAGE" = staging ]; then
  echo
  log_info "Vibe code here:   ssh $HOST, then cd $APP_DIR - the stack hot-reloads"
  log_info "Pull your work:   make deploystaging HOST=$HOST DOMAIN=$DOMAIN"
  log_info "Ship to prod:     make promote          (on this server)"
  log_info "                  make promote STAGING=$HOST [PROD=deploy@…]   (from your machine)"
fi
