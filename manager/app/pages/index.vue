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
const toast = useToast()
const { t } = useI18n()

const page = ref(1)

const client = reactive({ name: '', slug: '' })
const creating = ref(false)

// The slug follows the name until someone edits it.
function slugOf(name: string) {
  return name.toLowerCase().replaceAll('ł', 'l').normalize('NFD').replace(/[\u0300-\u036f]/g, '')
    .replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '')
}

function onName() {
  client.slug = slugOf(client.name)
}

async function createClient() {
  creating.value = true

  try {
    const created = await api.post<{ id: string }>('/api/manager/tenants', { ...client })
    await navigateTo(`/tenants/${created.id}`)
  } catch (error) {
    toast.error((error as Error).message)
  } finally {
    creating.value = false
  }
}

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

    <UiCard :title="t('manager.tenantCreate')" :description="t('manager.tenantCreateHint')">
      <form class="flex flex-wrap items-end gap-3" @submit.prevent="createClient">
        <UiField v-slot="field" :label="t('manager.tenantName')" class="min-w-64 flex-1">
          <UiInput
            :id="field.id"
            v-model="client.name"
            required
            data-testid="tenant-name"
            :described-by="field.describedBy"
            @input="onName"
          />
        </UiField>
        <UiField v-slot="field" :label="t('manager.slug')" class="min-w-48">
          <UiInput :id="field.id" v-model="client.slug" required data-testid="tenant-slug" :described-by="field.describedBy" />
        </UiField>
        <UiButton type="submit" variant="primary" :loading="creating" data-testid="tenant-create">
          {{ t('manager.tenantCreateSubmit') }}
        </UiButton>
      </form>
    </UiCard>

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
