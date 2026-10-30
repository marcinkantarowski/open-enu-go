# =============================================================================
#  OpenEnu Go - the single entry point for dev and deployment.
# =============================================================================
#  Every target carries a `## description` and `help` parses them, so the list
#  below can never drift from what is actually implemented. A target is listed
#  only once it works - an unimplemented target is an absent target, not a stub
#  that fails at 3am.
# =============================================================================

SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help
.NOTPARALLEL:

ROOT := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))
LIB    := $(ROOT)/scripts/lib
DEV    := $(ROOT)/scripts/dev
REMOTE := $(ROOT)/scripts/remote

-include .env
export

PROJECT_SLUG ?= open-enu-go
DOMAIN       ?= open-enu-go.local

# Go is native, never containerised: the inner loop has to be fast enough to be
# run after every edit. scripts/lib/go.sh finds a tarball install that is not
# on PATH, so the Makefile and every script agree on one binary.
GO ?= $(shell bash $(LIB)/go.sh 2>/dev/null || echo go)
export GO

# COMPOSE_FILE comes from .env: the dev file locally, dev + the staging override
# on a staging server. Never name the files here - see scripts/lib/compose.sh,
# which also explains the project's own files in docker/project/.
COMPOSE := docker compose --project-directory $(ROOT) --env-file $(ROOT)/.env \
           $(foreach f,$(subst :, ,$(or $(COMPOSE_FILE),docker/compose.dev.yml)),-f $(ROOT)/$(f) \
             $(addprefix -f ,$(wildcard $(ROOT)/docker/project/$(notdir $(f)))))

# The binary, inside the api container. `go run` rather than the path air
# built to, so this works while air is still compiling the first build.
OPENENU := $(COMPOSE) exec -T api go run ./cmd/openenu

# The exact @playwright/test version is the browser image's tag (compose reads
# PLAYWRIGHT_VERSION). One source, e2e/package.json; never written twice.
PLAYWRIGHT_VERSION := $(shell sed -n 's/.*"@playwright\/test": *"\([0-9][0-9.]*\)".*/\1/p' $(ROOT)/e2e/package.json)

# ═════════════════════════════════════════════════════════════════════════════
##@ Setup
# ═════════════════════════════════════════════════════════════════════════════

.PHONY: init
init: ## Rename the boilerplate into your project (NAME=myproject [DOMAIN=...] [GO_MODULE=...] [PLATFORM_SOURCE=... PLATFORM_REF=...])
	@$(DEV)/init.sh

.PHONY: platform-update
platform-update: ## Bring this project up to a newer platform, staged and uncommitted (FROM=<path|url> [REF=...] [BASE=...])
	@$(DEV)/platform-update.sh

.PHONY: check-tools
check-tools: ## Verify the host toolchain, split into needed-now and needed-later
	@$(DEV)/check-tools.sh

.PHONY: module
module: ## Scaffold a feature end to end: backend module, spec fragment + server, frontend layer, docs, spec stub, Task Router row (NAME=billing)
	@$(DEV)/module.sh

.PHONY: manager-module
manager-module: ## Give an existing module operator screens: a layer under manager/app/modules/ (NAME=billing)
	@$(DEV)/manager-module.sh

.PHONY: modules
modules: ## Show what module discovery resolved
	@cd $(ROOT)/backend && $(GO) run ./cmd/openenu modules

.PHONY: inventory
inventory: ## Regenerate .ai/inventory.json - what already exists, for whoever adds the next thing
	@cd $(ROOT)/backend && $(GO) run ./cmd/openenu inventory

.PHONY: inventory-check
inventory-check: ## Fail if .ai/inventory.json no longer matches the code (runs in arch)
	@cd $(ROOT)/backend && $(GO) run ./cmd/openenu inventory -check

.PHONY: mcp-list
mcp-list: ## Print the tools an MCP client would be offered
	@cd $(ROOT)/backend && $(GO) run ./cmd/openenu mcp -list

# ═════════════════════════════════════════════════════════════════════════════
##@ Dev stack
# ═════════════════════════════════════════════════════════════════════════════
#  Everything here needs Docker. Nothing in `make check` does.

.PHONY: builddev
builddev: check-tools env certs hosts up deps wait migrate smoke urls ## Bring the whole stack up from nothing (idempotent)

.PHONY: env
env: ## Create/repair .env and derive the Nuxt, e2e and project env files from it
	@$(LIB)/envgen.sh generate

.PHONY: env-check
env-check: ## Report .env drift against .env.example (read-only)
	@$(LIB)/envgen.sh check

.PHONY: certs
certs: ## Issue the locally-trusted wildcard certificate (and trust it on Windows under WSL2)
	@$(DEV)/certs.sh

.PHONY: hosts
hosts: ## Map every ${DOMAIN} host to 127.0.0.1 (Linux and, under WSL2, Windows)
	@$(DEV)/hosts.sh

.PHONY: edge
edge: ## Start the shared edge proxy (one per Docker host) and register this project with it
	@$(DEV)/edge.sh up

.PHONY: ports
ports: ## Check that no other project or process holds a host port this stack publishes (read-only)
	@$(DEV)/check-ports.sh

.PHONY: up
up: ports edge ## Build images if needed and start every container
	@$(COMPOSE) up -d --build --remove-orphans
	@$(DEV)/edge.sh attach

.PHONY: wait
wait: ## Block until postgres, redis, mercure, the API and the three apps serve, and the edge routes them
	@$(DEV)/wait.sh

.PHONY: deps
deps: ## Install the npm workspace inside the node container
	@$(DEV)/deps.sh

.PHONY: migrate
migrate: ## Apply pending migrations as the schema owner (DATABASE_ADMIN_URL)
	@$(OPENENU) migrate

.PHONY: smoke
smoke: ## Verify every host answers over trusted HTTPS, body included
	@$(DEV)/smoke.sh

.PHONY: urls
urls: ## Print the URL table
	@$(DEV)/urls.sh

.PHONY: down
down: ## Stop and remove containers (volumes survive; the shared edge keeps running)
	@$(DEV)/edge.sh detach
	@$(COMPOSE) down --remove-orphans

.PHONY: stop
stop: ## Stop containers without removing them
	@$(COMPOSE) stop

.PHONY: restart
restart: ## Restart every container
	@$(COMPOSE) restart

.PHONY: rebuild
rebuild: ## Rebuild images from scratch and restart
	@$(COMPOSE) build --pull --no-cache
	@$(COMPOSE) up -d --remove-orphans
	@$(DEV)/edge.sh attach

.PHONY: status
status: ## Show container status and health
	@$(COMPOSE) ps

# ═════════════════════════════════════════════════════════════════════════════
##@ Preflight
# ═════════════════════════════════════════════════════════════════════════════
#  Read-only, every one of them. They diagnose and they generate fixes; they
#  never change a DNS zone or a server as a side effect (deployment.md).

.PHONY: preflight
preflight: ## Everything a remote target needs, checked first: make preflight HOST=deploy@ip DOMAIN=x.com REPO=org/x
	@$(REMOTE)/preflight.sh "$(HOST)" "$(DOMAIN)" "$(REPO)" "$(or $(REF),main)"

.PHONY: dns-check
dns-check: ## Check DNS against the target and write importable fixes: make dns-check DOMAIN=x.com IP=1.2.3.4
	@$(REMOTE)/dns-check.sh "$(DOMAIN)" "$(or $(IP),$(HOST))" $(if $(TTL),--ttl $(TTL),) $(if $(DNS_MODE),--mode $(DNS_MODE),)

.PHONY: dns-apply
dns-apply: ## Apply the generated Cloudflare deltas - opt-in, never run by a build: CF_API_TOKEN=... make dns-apply DOMAIN=x.com
	@test -f .out/dns/$(DOMAIN)/cloudflare.sh || { echo "  run make dns-check first"; exit 1; }
	@bash .out/dns/$(DOMAIN)/cloudflare.sh --apply

.PHONY: deploy-key
deploy-key: ## Give a stage access to the repo (production read-only, STAGE=staging read-write): make deploy-key HOST=deploy@ip REPO=org/x [STAGE=staging]
	@$(REMOTE)/deploy-key.sh "$(HOST)" "$(REPO)" "$(or $(STAGE),production)"

# ═════════════════════════════════════════════════════════════════════════════
##@ Remote
# ═════════════════════════════════════════════════════════════════════════════

.PHONY: buildstaging
buildstaging: ## Bare server → staging (the dev stack, for vibe coding): make buildstaging HOST=deploy@ip DOMAIN=stg.x.com REPO=org/x EMAIL=you@x.com [ALLOW=ip,cidr]
	@$(REMOTE)/provision.sh "$(HOST)" "$(DOMAIN)" staging "$(REPO)" "$(or $(REF),main)"

# Production's target: HOST= DOMAIN= REPO= on the command line, else PROD_* in
# .env. Only the command line counts: an exported HOST must not pick a server.
typed     = $(if $(filter command line,$(origin $(1))),$($(1)))
P_HOST    = $(or $(call typed,HOST),$(PROD_HOST))
P_DOMAIN  = $(or $(call typed,DOMAIN),$(PROD_DOMAIN))
P_REPO    = $(or $(call typed,REPO),$(PROD_REPO))
need-prod = test -n "$(P_HOST)" && test -n "$(P_DOMAIN)" \
  || { echo "  no production target: set PROD_HOST and PROD_DOMAIN in .env, or pass HOST= DOMAIN="; exit 1; }

.PHONY: buildprod
buildprod: ## Bare server → production, one command; same server as staging or its own (refuses to finish without BACKUP_REMOTE)
	@$(need-prod)
	@$(REMOTE)/provision.sh "$(P_HOST)" "$(P_DOMAIN)" production "$(P_REPO)" "$(or $(REF),main)"

.PHONY: deploystaging
deploystaging: ## Pull a ref into staging - refuses to touch uncommitted work there: make deploystaging HOST=deploy@ip DOMAIN=stg.x.com [REF=main]
	@$(REMOTE)/deploy.sh "$(HOST)" "$(DOMAIN)" staging "$(or $(REF),main)"

.PHONY: deployprod
deployprod: ## Ship a ref to production (runs `make ci` first; brief downtime; target: PROD_HOST and PROD_DOMAIN in .env, or HOST= DOMAIN=). Usually: make promote
	@$(need-prod)
	@$(REMOTE)/deploy.sh "$(P_HOST)" "$(P_DOMAIN)" production "$(or $(REF),main)"

.PHONY: copyproddata
copyproddata: ## REPLACE the dev database with a copy of production's, real people included (target: PROD_HOST in .env, or HOST=)
	@$(need-prod)
	@$(REMOTE)/copyproddata.sh "$(P_HOST)"

.PHONY: promote
promote: ## Production runs exactly what staging runs: make promote (on staging) | make promote STAGING=deploy@ip [PROD=deploy@ip]
	@STAGING="$(STAGING)" PROD="$(PROD)" $(REMOTE)/promote.sh

.PHONY: promote-gate
promote-gate: arch test typecheck ## The gate `make promote` runs on staging before production moves

.PHONY: backup
backup: ## Take a production backup on the server now: make backup HOST=deploy@ip
	@ssh "$(HOST)" "cd /opt/$(PROJECT_SLUG)-prod && bash scripts/remote/backup.sh manual"

.PHONY: backup-verify
backup-verify: ## Restore the latest OFF-BOX backup into a scratch database and validate it
	@$(REMOTE)/backup-verify.sh "$(HOST)"

.PHONY: restore
restore: ## Put a backup back - destructive, asks for the database name: make restore HOST=deploy@ip [FILE=...]
	@$(REMOTE)/restore.sh "$(HOST)" "$(FILE)"

.PHONY: tunnel
tunnel: ## Forward a stage's database to localhost:15432 for one session: make tunnel HOST=deploy@ip [STAGE=staging]
	@# Postgres publishes no port on a server, so forward to the container itself.
	@ip=$$(ssh "$(HOST)" "docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' $(PROJECT_SLUG)-$(if $(filter staging,$(STAGE)),staging,prod)-postgres") \
	 && [ -n "$$ip" ] || { echo "  no $(PROJECT_SLUG)-$(if $(filter staging,$(STAGE)),staging,prod)-postgres container on $(HOST)"; exit 1; }; \
	 echo "  postgres://$(DB_USER)@127.0.0.1:15432/$(DB_NAME) - ctrl-c to close"; \
	 ssh -N -L 15432:$$ip:5432 "$(HOST)"

.PHONY: logs
logs: ## Follow logs from every container
	@$(COMPOSE) logs -f --tail=100

.PHONY: logs-api
logs-api: ## Follow the API's log (air's build output included)
	@$(COMPOSE) logs -f --tail=100 api

.PHONY: logs-worker
logs-worker: ## Follow the worker's and the scheduler's logs
	@$(COMPOSE) logs -f --tail=100 worker scheduler

.PHONY: logs-ui
logs-ui: ## Follow frontend, manager and landing logs
	@$(COMPOSE) logs -f --tail=50 frontend manager landing

.PHONY: logs-traefik
logs-traefik: ## Follow the shared edge proxy's logs (every project on this host)
	@$(DEV)/edge.sh logs

.PHONY: shell
shell: ## Open a shell in the API container
	@$(COMPOSE) exec api sh

.PHONY: shell-ui
shell-ui: ## Open a shell in the frontend container
	@$(COMPOSE) exec frontend sh

.PHONY: psql
psql: ## Open psql on the development database, as the owner
	@$(COMPOSE) exec postgres psql -U $(DB_USER) $(DB_NAME)

.PHONY: redis
redis: ## Open redis-cli
	@$(COMPOSE) exec redis redis-cli -a $(REDIS_PASSWORD) --no-auth-warning

# ═════════════════════════════════════════════════════════════════════════════
##@ Frontends
# ═════════════════════════════════════════════════════════════════════════════

.PHONY: types
types: ## Regenerate ui-kit/types/api.d.ts from backend/openapi.json and stamp it with the spec's hash (gen-check reads the stamp)
	@$(COMPOSE) run --rm --no-deps --entrypoint sh frontend -c 'npx -w @open-enu/ui-kit openapi-typescript ../backend/openapi.json -o types/api.d.ts'
	@$(DEV)/check-types.sh --stamp

.PHONY: typecheck
typecheck: ## Type-check all three Nuxt apps against ui-kit/types/api.d.ts
	@$(DEV)/typecheck.sh

.PHONY: db-reset
db-reset: ## Empty the dev database and migrate it (the way back from copyproddata)
	@$(DEV)/db-reset.sh

.PHONY: e2e
e2e: ## Browser tests on their own fresh database, ${DB_NAME}_e2e; the stack goes back afterwards (SPEC=tests/x.spec.ts)
	@test -n "$(PLAYWRIGHT_VERSION)" || { echo "  no exact @playwright/test version in e2e/package.json" >&2; exit 1; }
	@PLAYWRIGHT_VERSION=$(PLAYWRIGHT_VERSION) $(DEV)/e2e.sh $(SPEC)

# ═════════════════════════════════════════════════════════════════════════════
##@ Quality
# ═════════════════════════════════════════════════════════════════════════════

.PHONY: check
check: ## THE INNER LOOP - run after every edit (budget: 60s, no Docker)
	@$(DEV)/check.sh

.PHONY: ci
ci: check-tools arch selftest test route-coverage typecheck smoke e2e ## The full gate; what CI runs - needs Docker and the dev stack up

.PHONY: test-security
test-security: ## The suite that proves isolation and credential handling (needs Docker)
	@cd $(ROOT)/backend && $(GO) test ./internal/security/...

.PHONY: arch
arch: vet analyzers gen-check inventory-check docs-check i18n-check typography-check versions-check agents-budget shell-check prod-check dead-code ## Every architectural guardrail

.PHONY: analyzers
analyzers: ## The go/analysis guardrails (writeboundary, thincontroller, rawsql, platformservice, flagdeclared, rawstring, textlen, tableowner, impersonationguard) over both modules
	@cd $(ROOT)/backend && $(GO) run github.com/open-enu/kernel/analysis/cmd/arch ./...
	@cd $(ROOT)/backend/kernel && $(GO) run ./analysis/cmd/arch ./...

.PHONY: generate
generate: ## Regenerate every module's server, the module registry and the merged openapi.json
	@cd $(ROOT)/backend && for m in internal/module/*/; do [ -f "$$m/openapi.json" ] && (cd "$$m" && $(GO) generate ./) || true; done
	@cd $(ROOT)/backend && $(GO) generate ./cmd/openenu

.PHONY: gen-check
gen-check: ## Fail if any generated file (api.gen.go, module_registry_gen.go, openapi.json, acl_gen.go, ui-kit/types/api.d.ts) is stale
	@cd $(ROOT)/backend/cmd/openenu && $(GO) run ../../tools/genregistry -check && $(GO) run ../../tools/genspec -check && $(GO) run ../../tools/genacl -check
	@cd $(ROOT)/backend && for m in internal/module/*/; do [ -f "$$m/openapi.json" ] && (cd "$$m" && $(GO) run ../../../tools/genapi -check) || true; done
	@$(DEV)/check-types.sh

.PHONY: openapi-check
openapi-check: ## Fail if backend/openapi.json no longer matches the module fragments
	@cd $(ROOT)/backend/cmd/openenu && $(GO) run ../../tools/genspec -check

.PHONY: selftest
selftest: ## Prove the guardrails fail when broken, on a throwaway copy
	@$(DEV)/selftest.sh

.PHONY: fmt
fmt: ## gofmt both modules in place
	@cd $(ROOT)/backend/kernel && $(dir $(GO))gofmt -w .
	@cd $(ROOT)/backend && $(dir $(GO))gofmt -w $$(ls -d */ | grep -v '^kernel/') *.go 2>/dev/null || true

.PHONY: vet
vet: ## go vet both modules
	@cd $(ROOT)/backend/kernel && $(GO) vet ./...
	@cd $(ROOT)/backend && $(GO) vet ./...

.PHONY: build
build: ## Build the openenu binary into backend/bin/
	@cd $(ROOT)/backend && CGO_ENABLED=0 $(GO) build -trimpath -o bin/openenu ./cmd/openenu

.PHONY: test
# Two Go modules are two testcontainers sessions back to back. With the reaper on, the first
# session's reaper removes the shared containers ten seconds after that session ends - while
# the second has already reused them, and every test in flight sees "connection reset". So the
# reaper is off for the pair and the containers are removed here, success or failure. The names
# are testdb.go's (kernel/testdb), prefixed with the slug so that two projects on one Docker
# host never reuse, or remove, each other's.
export TESTDB_CONTAINER_PREFIX ?= $(PROJECT_SLUG)
TEST_CONTAINERS := $(TESTDB_CONTAINER_PREFIX)-test-postgres $(TESTDB_CONTAINER_PREFIX)-test-redis

# ROUTE_TRACE: every request a test makes appends its matched route to this file
# (kernel/httpx/trace.go); `make route-coverage` subtracts it from the spec. A stale
# trace would credit this run with routes an earlier one hit, so it is removed first.
ROUTE_TRACE := $(ROOT)/backend/.route-trace

.PHONY: test
test: ## Every Go test, both modules (tests needing Postgres/Redis start them with testcontainers); writes the route trace
	@rm -f $(ROUTE_TRACE); status=0; \
	  (cd $(ROOT)/backend/kernel && TESTCONTAINERS_RYUK_DISABLED=true $(GO) test ./...) && \
	  (cd $(ROOT)/backend && TESTCONTAINERS_RYUK_DISABLED=true ROUTE_TRACE=$(ROUTE_TRACE) $(GO) test -count=1 ./...) || status=$$?; \
	  docker rm -f $(TEST_CONTAINERS) >/dev/null 2>&1; \
	  exit $$status

.PHONY: route-coverage
route-coverage: ## Every operation in the spec was reached by a test - reads the trace `make test` wrote
	@ROUTE_TRACE=$(ROUTE_TRACE) $(DEV)/check-route-coverage.sh

.PHONY: dead-code
dead-code: ## Unused TypeScript and unreachable application Go (blocking); unreached kernel exports (advisory)
	@$(DEV)/check-dead-code.sh

.PHONY: test-kernel
test-kernel: ## The framework's own tests
	@cd $(ROOT)/backend/kernel && $(GO) test ./...

.PHONY: docs-check
docs-check: ## Verify docs link correctly, records are indexed, MODULE.md names the contract
	@$(DEV)/check-docs.sh

.PHONY: i18n-check
i18n-check: ## Fail when a translation key or a landing page exists in one locale and not the other
	@$(DEV)/check-i18n.sh

.PHONY: typography-check
typography-check: ## Fail on a long dash (U+2014) anywhere - this project writes a plain hyphen
	@$(DEV)/check-typography.sh

.PHONY: versions-check
versions-check: ## Fail when Postgres, Redis, Mercure, Go or Node is a different version in different files
	@$(DEV)/check-versions.sh

.PHONY: agents-budget
agents-budget: ## Check AGENTS.md / MODULE.md / module and kernel size budgets
	@$(DEV)/agents-budget.sh

.PHONY: shell-check
shell-check: ## ShellCheck every script - the shell is what runs against real servers (needs Docker)
	@$(DEV)/check-shell.sh

.PHONY: prod-check
prod-check: ## Verify the production and staging stacks' invariants: no source mounts, no DB port, pinned images, allowlisted routers
	@$(DEV)/check-prod-compose.sh

# ═════════════════════════════════════════════════════════════════════════════
##@ Help
# ═════════════════════════════════════════════════════════════════════════════

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage: make \033[36m<target>\033[0m\n"} \
	  /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 } \
	  /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
	@echo
