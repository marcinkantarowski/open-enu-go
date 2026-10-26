<script setup lang="ts">
/**
 * The audit trail across every tenant.
 *
 * The reason this exists separately from the tenant-facing one: an operator
 * investigating an incident needs to look at a tenant they are not a member of,
 * and needs to see impersonated actions attributed to the operator who performed
 * them rather than to the customer.
 */
interface AuditEntry {
  id: string
  action: string
  tenantId: string | null
  actorId: string | null
  onBehalfOfId: string | null
  requestId: string | null
  succeeded: boolean
  recordedAt: string
}

const api = useApi()
const { t } = useI18n()

const tenantId = ref('')

const { data, pending, refresh } = await useAsyncData(
  'manager-audit',
  () => api.get<{ items: AuditEntry[] }>('/api/manager/audit', {
    query: { tenantId: tenantId.value || undefined, limit: 200 },
  }),
)
</script>

<template>
  <div class="flex flex-col gap-6">
    <div class="flex items-end justify-between gap-4">
      <div>
        <h1 class="text-lg font-semibold text-fg">{{ t('manager.audit') }}</h1>
        <p class="text-sm text-fg-muted">{{ t('manager.auditHint') }}</p>
      </div>

      <form class="flex items-end gap-2" @submit.prevent="refresh()">
        <UiField v-slot="field" :label="t('manager.filterTenant')">
          <UiInput :id="field.id" v-model="tenantId" placeholder="UUID" :described-by="field.describedBy" />
        </UiField>
        <UiButton type="submit">{{ t('common.search') }}</UiButton>
      </form>
    </div>

    <UiCard>
      <UiTable
        :columns="[
          { key: 'recordedAt', label: t('manager.when') },
          { key: 'action', label: t('manager.action') },
          { key: 'tenantId', label: t('manager.tenant') },
          { key: 'actorId', label: t('manager.actor') },
          { key: 'succeeded', label: t('manager.outcome') },
          { key: 'requestId', label: t('manager.request') },
        ]"
        :rows="data?.items ?? []"
        :loading="pending"
        :empty-title="t('manager.noAudit')"
      >
        <template #cell-action="{ row }">
          <span class="font-mono text-xs">{{ row.action }}</span>
        </template>

        <template #cell-actorId="{ row }">
          <span class="font-mono text-xs">{{ row.actorId ?? '-' }}</span>
          <!-- The distinction the tenant-facing view cannot make: which of these
               changes an operator made while wearing someone else's face. -->
          <UiBadge v-if="row.onBehalfOfId" tone="warning" class="ml-1">{{ t('manager.impersonated') }}</UiBadge>
        </template>

        <template #cell-succeeded="{ row }">
          <UiBadge :tone="row.succeeded ? 'success' : 'danger'">
            {{ row.succeeded ? t('manager.outcomeOk') : t('manager.outcomeFailed') }}
          </UiBadge>
        </template>

        <template #cell-requestId="{ row }">
          <span class="font-mono text-[11px] text-fg-muted">{{ row.requestId ?? '-' }}</span>
        </template>
      </UiTable>
    </UiCard>
  </div>
</template>
