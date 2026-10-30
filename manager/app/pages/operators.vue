<script setup lang="ts">
/**
 * Who can sign in here. Read only: an operator is created on the server, on
 * purpose (`openenu manager create`), and there is no signup endpoint.
 */
interface OperatorRow {
  id: string
  email: string
  displayName: string | null
  roles: string[]
  active: boolean
  lastLoginAt: string | null
}

const api = useApi()
const { t, locale } = useI18n()

const { data, pending } = await useAsyncData(
  'manager-operators',
  () => api.get<{ items: OperatorRow[] }>('/api/manager/operators'),
)
</script>

<template>
  <div class="flex flex-col gap-6">
    <div>
      <h1 class="text-lg font-semibold text-fg">{{ t('manager.operators') }}</h1>
      <p class="text-sm text-fg-muted">{{ t('manager.operatorsHint') }}</p>
    </div>

    <UiCard>
      <UiTable
        :columns="[
          { key: 'displayName', label: t('manager.name') },
          { key: 'email', label: t('auth.email') },
          { key: 'roles', label: t('manager.role') },
          { key: 'active', label: t('manager.status') },
          { key: 'lastLoginAt', label: t('manager.lastLogin') },
        ]"
        :rows="data?.items ?? []"
        :loading="pending"
        :empty-title="t('manager.noOperators')"
      >
        <template #cell-email="{ row }">
          <span data-testid="operator-email">{{ row.email }}</span>
        </template>

        <template #cell-roles="{ row }">
          {{ row.roles.includes('ROLE_PLATFORM_SUPER') ? t('manager.operatorSuper') : t('manager.operatorPlain') }}
        </template>

        <template #cell-active="{ row }">
          <UiBadge :tone="row.active ? 'success' : 'neutral'">
            {{ row.active ? t('manager.active') : t('manager.inactive') }}
          </UiBadge>
        </template>

        <template #cell-lastLoginAt="{ row }">
          {{ row.lastLoginAt ? new Date(row.lastLoginAt).toLocaleString(locale) : t('manager.never') }}
        </template>
      </UiTable>
    </UiCard>
  </div>
</template>
