<script setup lang="ts">
/**
 * Whether the live stream is actually connected.
 *
 * Small, but load-bearing during development: without it a page that has
 * silently lost its SSE connection is indistinguishable from one where nothing
 * has happened yet.
 */
const { status } = useMercure()
const { t } = useI18n()

// `idle` has no label on purpose: nothing wants a stream, so there is nothing
// to report - and "Reconnecting" there would be a lie.
const labels = {
  connecting: 'realtime.connecting',
  live: 'realtime.live',
  reconnecting: 'realtime.reconnecting',
} as const
</script>

<template>
  <span
    v-if="status !== 'idle'"
    class="inline-flex items-center gap-1.5 text-xs text-fg-muted"
    data-testid="realtime-indicator"
    :data-status="status"
  >
    <span
      class="size-2 rounded-full"
      :class="status === 'live' ? 'bg-success' : 'bg-warning animate-pulse'"
      aria-hidden="true"
    />
    {{ t(labels[status]) }}
  </span>
</template>
