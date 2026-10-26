# A worker runs the code it started with

A process that boots once and runs for a long time runs the code it booted with. For it, "I
saved the file" and "the code is running" are different statements, and nothing says which
one holds.

## The trap
The api inserts a job and the worker runs it: two processes of one binary, both wired by the
module's `Mount`, so neither can disagree with the other about a type
(`internal/module/example/internal/mount.go`):

```go
outbox.Handle(svc.Outbox, archiveWorker{bus: svc.Bus, progress: svc.Progress}.run)
```

A job kind added in an edit exists in each process only once that process has been rebuilt
and restarted. Until the worker has been, the request that queues the job answers `202` with a
job id, the progress bar appears, and nothing else ever does. The module's tests are green,
because a test binary is compiled from the code on disk the moment it runs. The browser test
times out waiting for a result.

An *edited* job is worse: nothing fails, the old behaviour simply continues, and the person
debugging it is reading code that is not the code being executed.

The same shape exists in the frontend: editing a file inside a Nuxt layer hot-reloads, adding
a layer does not, because the list of layers is read once at startup.

## The rule
Anything that boots once and runs for a long time is restarted by the thing that changes its
code, not by the person who changed it.

## Where it is held
- In the dev stack `api`, `worker` and `scheduler` run under air (`docker/compose.dev.yml`,
  `backend/.air.toml`), which polls the mounted source, rebuilds on a `.go`, `.sql`, `.json`,
  `.yaml` or `.toml` change, and restarts the process with SIGTERM first, so River drains the
  job in hand.
- `make module` restarts the frontend, because a new layer is only discovered at startup.
- Production is the opposite case and stays that way: an image's binary cannot change
  underneath a worker, so nothing there watches anything. A release's `compose up -d`
  recreates the containers whose image changed, and `make prod-check` refuses a production
  container that runs air or a backend built from the `dev` target.

## Where it still bites in dev
- `stop_on_error = false` in `backend/.air.toml` keeps the previous binary running when a build
  fails, so an edit that does not compile leaves the worker on the old code. The build error
  is in `make logs-worker`.
- air restarts the process, not the container: a change to the environment reaches the backend
  only when the container is recreated (`make up`).

## How to recognise it
A change that works in a test and not in the running stack, where the difference between the
two is the age of a process. Read the process's last rebuild in its log (`make logs-api`,
`make logs-worker`) - or, outside dev, the container's age in `make status` - before reading
the code again.
