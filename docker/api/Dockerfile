# =============================================================================
#  The Go backend - one image, three processes: api, worker, scheduler.
# =============================================================================
#  Build context is the REPOSITORY ROOT (.dockerignore lives there), so the
#  kernel module (backend/kernel) is inside the context for the prod build.
#
#  Two shapes. `dev` bind-mounts backend/ and runs the binary under air, which
#  rebuilds it when a file changes - into /tmp, so the container never writes
#  into the source tree and needs no uid remap. `prod` is one static binary on
#  a distroless base: no shell, no package manager, nothing to patch but Go.
#  `openenu worker` and `openenu scheduler` are the same image with a
#  different argument (.ai/platform/docs/dev-stack.md).
# =============================================================================

FROM golang:1.27.3-alpine AS base
RUN apk add --no-cache ca-certificates tzdata git
WORKDIR /app

# ─────────────────────────────────────────────────────────────────────────────
FROM base AS dev

# Pinned: a reloader that changes under a developer is a reloader nobody
# trusts. The module cache and the build cache are named volumes in compose,
# shared by api, worker and scheduler, so the three rebuilds after one edit
# compile each package once.
RUN go install github.com/air-verse/air@v1.67.4

# The source is a bind mount with no .git inside the container's view of it;
# without this every build stamps "error obtaining VCS status" into the log.
ENV GOFLAGS=-buildvcs=false \
    AIR_WD=/app

EXPOSE 8080
# Per service, compose appends `-- <command>`: air passes what follows `--` to
# the binary it built. The default is the API.
CMD ["air", "-c", ".air.toml", "--", "api"]

# ─────────────────────────────────────────────────────────────────────────────
FROM base AS build
COPY backend/ /app/
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /openenu ./cmd/openenu

FROM gcr.io/distroless/static-debian13:nonroot AS prod
COPY --from=build /openenu /openenu
USER nonroot
EXPOSE 8080
ENTRYPOINT ["/openenu"]
CMD ["api"]
