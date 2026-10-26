<script setup lang="ts">
/**
 * "It didn't happen" - answered by two numbers.
 *
 * How much is waiting, and how much has failed. Everything in this system that
 * happens after a response goes through these queues (ADR-0015), so a growing
 * depth or a non-zero failed count is the first thing to look at.
 */
const api = useApi()
const { t } = useI18n()

const { data, pending, refresh } = await useAsyncData(
  'worker-status',
  () => api.get<{ queues: Record<string, number>, failed: number }>('/api/manager/workers'),
)

// Polled rather than streamed: this is an operator staring at a screen during an
// incident, and a ten-second refresh is both sufficient and one less moving part
// than a subscription that could itself be the thing that is broken.
const timer = setInterval(() => void refresh(), 10_000)
onBeforeUnmount(() => clearInterval(timer))

const rows = computed(() =>
  Object.entries(data.value?.queues ?? {}).map(([queue, depth]) => ({ queue, depth })),
)
</script>

<template>
  <div class="flex flex-col gap-6">
    <div>
      <h1 class="text-lg font-semibold text-fg">{{ t('manager.workers') }}</h1>
      <p class="text-sm text-fg-muted">{{ t('manager.workersHint') }}</p>
    </div>

    <div class="grid gap-4 md:grid-cols-2">
      <UiCard :title="t('manager.queues')">
        <UiTable
          :columns="[
            { key: 'queue', label: t('manager.queue') },
            { key: 'depth', label: t('manager.waiting'), numeric: true },
          ]"
          :rows="rows"
          row-key="queue"
          :loading="pending"
          :empty-title="t('manager.noQueues')"
        />
      </UiCard>

      <UiCard :title="t('manager.failed')">
        <p
          class="text-4xl font-semibold tabular-nums"
          :class="(data?.failed ?? 0) > 0 ? 'text-danger' : 'text-success'"
          data-testid="failed-count"
        >
          {{ data?.failed ?? 0 }}
        </p>
        <p class="mt-2 text-xs text-fg-muted">{{ t('manager.failedHint') }}</p>
      </UiCard>
    </div>
  </div>
</template>
