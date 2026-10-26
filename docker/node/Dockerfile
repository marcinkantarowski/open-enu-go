# =============================================================================
#  Nuxt apps - one image, three containers (frontend, manager, landing).
# =============================================================================
#  Node 26 regardless of what the host has; the host's node is only ever a
#  convenience for the IDE (.ai/platform/docs/dev-stack.md).
# =============================================================================

FROM node:26-alpine AS base
RUN apk add --no-cache libc6-compat
WORKDIR /app

FROM base AS dev
ENV NUXT_TELEMETRY_DISABLED=1
RUN apk add --no-cache util-linux
COPY docker/node/entrypoint.sh /usr/local/bin/entrypoint
ENTRYPOINT ["/usr/local/bin/entrypoint"]
# Source and node_modules are mounted by compose; the working directory and the
# command are set per-service so one image serves all three apps.
EXPOSE 3000
CMD ["npm", "run", "dev"]

# ─────────────────────────────────────────────────────────────────────────────
FROM base AS prod

# One image per app, selected by the APP build argument. Nuxt compiles a
# self-contained server into .output, so the runtime needs neither node_modules
# nor the source - which is most of the image size and all of the source.
ARG APP=frontend

ENV NUXT_TELEMETRY_DISABLED=1 \
    NODE_ENV=production

# The whole workspace is copied because the apps EXTEND @open-enu/ui-kit as a
# workspace package: building one of them needs the layer's source present, and
# npm resolves it through the root lockfile.
COPY package.json package-lock.json ./
COPY ui-kit ./ui-kit
COPY frontend ./frontend
COPY manager ./manager
COPY landing ./landing

# `npm ci` and not `install`: the lockfile decides, and a deploy that silently
# resolved a different dependency tree than CI tested is not a deploy.
RUN npm ci --no-audit --no-fund \
 && npm run build --workspace "$APP" \
 && mkdir -p /srv \
 && cp -r "$APP/.output" /srv/output \
 && rm -rf /app /root/.npm

WORKDIR /srv
USER node

ENV PORT=3000 HOST=0.0.0.0
EXPOSE 3000
ENTRYPOINT []
CMD ["node", "output/server/index.mjs"]
