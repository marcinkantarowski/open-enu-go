<script setup lang="ts">
/**
 * Every tenant on the platform.
 *
 * The `tenant` table is not under row-level security - it is the row a scope
 * names - so what keeps this list from a tenant's browser is the realm: the
 * endpoint refuses any token that is not `aud: manager` (ADR-0019). Every
 * cross-tenant read behind it goes through a contract that lifts isolation by
 * name (ADR-0009).
 */
interface TenantRow {
  id: string
  slug: string
  name: string
  status: string
  members: number
  consultants: string[]
  createdAt?: string
}

const api = useApi()
const { t } = useI18n()

const page = ref(1)

const { data, pending } = await useAsyncData(
  'manager-tenants',
  () => api.get<{ items: TenantRow[], meta: { total: number, page: number, size: number } }>(
    '/api/manager/tenants',
    { query: { page: page.value, size: 25 } },
  ),
  { watch: [page] },
)

const pages = computed(() => Math.max(1, Math.ceil((data.value?.meta.total ?? 0) / (data.value?.meta.size ?? 25))))

const tones: Record<string, 'success' | 'warning' | 'danger' | 'neutral'> = {
  active: 'success',
  pending: 'warning',
  suspended: 'danger',
}
</script>

<template>
  <div class="flex flex-col gap-6">
    <div>
      <h1 class="text-lg font-semibold text-fg">{{ t('manager.tenants') }}</h1>
      <p class="text-sm text-fg-muted">{{ t('manager.tenantsHint') }}</p>
    </div>

    <UiCard>
      <UiTable
        :columns="[
          { key: 'name', label: t('manager.name') },
          { key: 'slug', label: t('manager.slug') },
          { key: 'status', label: t('manager.status') },
          { key: 'consultants', label: t('manager.consultants') },
          { key: 'members', label: t('manager.members'), numeric: true },
        ]"
        :rows="data?.items ?? []"
        :loading="pending"
        :empty-title="t('manager.noTenants')"
      >
        <template #cell-name="{ row }">
          <NuxtLink :to="`/tenants/${row.id}`" class="text-brand-700 hover:underline" data-testid="tenant-link">
            {{ row.name }}
          </NuxtLink>
        </template>

        <template #cell-consultants="{ row }">
          <span data-testid="tenant-consultants">{{ row.consultants.join(', ') || t('manager.consultantNone') }}</span>
        </template>

        <template #cell-status="{ row }">
          <UiBadge :tone="tones[String(row.status)] ?? 'neutral'">{{ row.status }}</UiBadge>
        </template>

      </UiTable>

      <template #footer>
        <div class="flex items-center justify-between text-xs text-fg-muted">
          <span>{{ t('common.page', { page, pages }) }}</span>
          <div class="flex gap-2">
            <UiButton size="sm" :disabled="page <= 1" @click="page--">{{ t('common.previous') }}</UiButton>
            <UiButton size="sm" :disabled="page >= pages" @click="page++">{{ t('common.next') }}</UiButton>
          </div>
        </div>
      </template>
    </UiCard>
  </div>
</template>
