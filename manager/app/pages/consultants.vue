<script setup lang="ts">
/**
 * Who serves which client (ADR-0028). A consultant is a membership, so this is
 * read across workspaces; assigning one happens on the client's page.
 */
interface ConsultantRow {
  id: string
  email: string
  displayName: string | null
  clients: { tenantId: string, name: string }[]
}

const api = useApi()
const { t } = useI18n()

const { data, pending } = await useAsyncData(
  'manager-consultants',
  () => api.get<{ items: ConsultantRow[] }>('/api/manager/consultants'),
)
</script>

<template>
  <div class="flex flex-col gap-6">
    <div>
      <h1 class="text-lg font-semibold text-fg">{{ t('manager.consultants') }}</h1>
      <p class="text-sm text-fg-muted">{{ t('manager.consultantsHint') }}</p>
    </div>

    <UiCard>
      <UiTable
        :columns="[
          { key: 'displayName', label: t('manager.name') },
          { key: 'email', label: t('auth.email') },
          { key: 'clients', label: t('manager.consultantClients') },
        ]"
        :rows="data?.items ?? []"
        :loading="pending"
        :empty-title="t('manager.noConsultants')"
      >
        <template #cell-email="{ row }">
          <span data-testid="consultant-row">{{ row.email }}</span>
        </template>

        <template #cell-clients="{ row }">
          <span class="flex flex-wrap gap-x-3 gap-y-1">
            <NuxtLink
              v-for="client in row.clients"
              :key="client.tenantId"
              :to="`/tenants/${client.tenantId}`"
              class="text-brand-700 hover:underline"
            >
              {{ client.name }}
            </NuxtLink>
          </span>
        </template>
      </UiTable>
    </UiCard>
  </div>
</template>
