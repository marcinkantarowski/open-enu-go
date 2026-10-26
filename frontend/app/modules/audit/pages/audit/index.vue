<script setup lang="ts">
definePageMeta({ middleware: 'permission', permission: 'audit.view' })

/**
 * What happened in this workspace.
 *
 * Every command the bus dispatched, with before/after snapshots. Scoped to the
 * caller's tenant by the controller rather than by the query filter - the entity
 * is deliberately unscoped so an operator can investigate across tenants, and the
 * restriction belongs where the authority is decided.
 */
interface AuditEntry {
  id: string
  action: string
  actorId: string | null
  onBehalfOfId: string | null
  subjectId: string | null
  requestId: string | null
  succeeded: boolean
  failureReason: string | null
  before: Record<string, unknown> | null
  after: Record<string, unknown> | null
  recordedAt: string
}

const api = useApi()
const { t } = useI18n()

const { data, pending } = await useAsyncData('audit', () => api.get<{ items: AuditEntry[] }>('/api/audit'))

const expanded = ref<string | null>(null)
</script>

<template>
  <div class="flex flex-col gap-6">
    <div>
      <h1 class="text-lg font-semibold text-fg">{{ t('audit.title') }}</h1>
      <p class="text-sm text-fg-muted">{{ t('audit.subtitle') }}</p>
    </div>

    <UiCard>
      <UiTable
        :columns="[
          { key: 'recordedAt', label: t('audit.when') },
          { key: 'action', label: t('audit.action') },
          { key: 'actorId', label: t('audit.actor') },
          { key: 'succeeded', label: t('audit.outcome') },
          { key: 'details', label: '' },
        ]"
        :rows="data?.items ?? []"
        :loading="pending"
        :empty-title="t('audit.empty')"
      >
        <template #cell-action="{ row }">
          <span class="font-mono text-xs">{{ row.action }}</span>
        </template>

        <template #cell-actor="{ row }">
          <!-- Both identities when one is acting for another: an impersonated
               change must never read as the customer's own (ADR-0020). -->
          <span class="font-mono text-xs">{{ row.actorId ?? '-' }}</span>
          <span v-if="row.onBehalfOfId" class="ml-1 text-xs text-warning">
            {{ t('audit.onBehalfOf') }}
          </span>
        </template>

        <template #cell-succeeded="{ row }">
          <UiBadge :tone="row.succeeded ? 'success' : 'danger'">
            {{ row.succeeded ? t('audit.ok') : t('audit.failed') }}
          </UiBadge>
        </template>

        <template #cell-details="{ row }">
          <UiButton
            variant="ghost"
            size="sm"
            @click="expanded = expanded === row.id ? null : String(row.id)"
          >
            {{ expanded === row.id ? t('audit.hide') : t('audit.show') }}
          </UiButton>
        </template>
      </UiTable>

      <div v-if="expanded" class="mt-4 grid gap-4 md:grid-cols-2">
        <div v-for="side in ['before', 'after'] as const" :key="side">
          <p class="mb-1 text-xs font-medium uppercase tracking-wide text-fg-muted">{{ t(`audit.${side}`) }}</p>
          <pre class="overflow-x-auto rounded-lg bg-surface-sunken p-3 font-mono text-[11px] text-fg">{{
            JSON.stringify(data?.items.find(i => i.id === expanded)?.[side] ?? null, null, 2)
          }}</pre>
        </div>
      </div>
    </UiCard>
  </div>
</template>
