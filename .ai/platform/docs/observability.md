# Observability

What this platform ships, what it deliberately does not, and how to turn the rest on.

## What is already here

**Structured logs with correlation.** Every process logs JSON lines through `slog`, and
every line logged with a context carries what that context knows (`kernel/logctx`):

| Field | Where it comes from |
|---|---|
| `request_id` | the `X-Request-Id` the response echoes - in the worker, the one the event or job carried from the request that queued it |
| `tenant_id` | the tenant in scope |
| `user_id` / `api_key_id` / `operator_id` | the caller, by kind; `impersonator_id` too when an operator wears a member's session |
| `command` | the audit action of the command whose handler is running |

A module adds its own through `Module.LogFields`. Only lines logged *with* a context get
them - `slog.InfoContext(ctx, ...)`, `log.LogAttrs(ctx, ...)` - so log that way. The request
id is what a user quotes when reporting a problem, and what ties a browser error to a line
the worker wrote twenty minutes later; the audit trail stores it too, for a job's entries as
for a request's.

**Mail, captured.** Mailpit takes every outbound message in dev at `https://mail.${DOMAIN}`.

**Queue depth and dead letters.** The operator console's Workers page
(`GET /api/manager/workers`) shows what is waiting in River and what has failed. A failed
event or job is retried five times, then kept in the `discarded` state with its errors.

**Liveness of the worker and the scheduler.** Neither serves HTTP, so each touches a
heartbeat file - `HEARTBEAT_FILE`, `/tmp/openenu-heartbeat` when unset - every 30 seconds
while it is healthy (`kernel/heartbeat`). The worker beats while River is still running and
the database answers a ping; the scheduler beats from its own loop, between tasks, so a task
that hangs stops the beat too. `openenu health -heartbeat /tmp/openenu-heartbeat [-max-age 2m]`
is the healthcheck: one line on stdout and exit 0 when the file is younger than `-max-age`
(default 2m, four missed beats), one line on stderr and exit 1 when it is older or missing.
The production image is distroless, so the binary is `/openenu` and the compose test is
`["CMD", "/openenu", "health", "-heartbeat", "/tmp/openenu-heartbeat"]`. A daily task that
runs longer than `-max-age` marks the scheduler unhealthy while it runs.

**The API's two probes** (`kernel/httpx/health.go`). `GET /health` is liveness: public,
touches nothing, answers `{"status":"ok","stage":...}` and no version - Traefik's load
balancer check and the container's `openenu health` poll it. `GET /health/deep` is
readiness: it runs every registered check (Postgres today) with a two-second timeout and
answers 503 naming the one that failed, never the driver's message. Because each call
pings the database it is not public: it answers a request whose `X-Health-Token` header
equals `HEALTH_TOKEN` (compared in constant time), or - only when `HEALTH_TOKEN` is empty -
one from a loopback or private address with no public hop in `X-Forwarded-For`. Anyone
else gets the same 404 as a route that does not exist. `make env` generates the token;
`make smoke` and `scripts/remote/release.sh` read it from `.env`, and an external uptime
monitor sends it as a header:
`curl -H "X-Health-Token: $HEALTH_TOKEN" https://api.$DOMAIN/health/deep`.

**The scheduler's output** goes to stdout like every other container's, so `make logs` and
`make logs-worker` are the whole story.

## Error tracking and traces

Not installed: a boilerplate that ships a vendor client nobody configured
adds weight to every project built from it, and a production dependency is an `Ask First`.
`.env.example` names no DSN for them, because nothing would read it.

When a project wants them:

- **Error tracking** (Sentry's Go SDK, or any): report from `httpx.Recover` and from the
  outbox's failed-job path, send `request_id` as a tag - without it an issue and a log line
  about the same failure cannot be joined - and scrub the encrypted columns and the refresh
  cookie: an error report is an outbound copy of application state.
- **Traces** (OpenTelemetry): earn their keep once there is a second service to trace
  between. The span boundaries worth instrumenting first are the seams this design names:
  the command bus, the outbox worker, and webhook delivery.

## What to reach for, by question

| Question | Where |
|---|---|
| What happened to *this* request? | `X-Request-Id` from the response, then `make logs-api` and `make logs-worker`, filtered on it |
| Who did it, and what changed? | the audit trail - `request_id` is on every entry, a job's included |
| Did the email send, and in which language? | `https://mail.${DOMAIN}` |
| Is anything stuck? | operator console → Workers |
| Did we deliver that webhook? | Settings → Webhooks → Deliveries, per attempt with the response code |
