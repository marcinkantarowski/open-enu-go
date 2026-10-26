<script setup lang="ts">
definePageMeta({ middleware: 'permission', permission: 'settings.view' })

/**
 * The workspace's own settings.
 *
 * Only entries the platform marks `tenantEditable` can be changed here. The rest
 * are operator kill switches, shown deliberately rather than hidden: "why can I
 * not turn this on?" is answerable, while a missing row is not.
 */
interface Setting {
  identifier: string
  name: string
  description: string | null
  type: string
  tenantEditable: boolean
  effectiveValue: unknown
  overridden: boolean
}

const api = useApi()
const auth = useAuthStore()
const toast = useToast()
const { t } = useI18n()

const { data, pending, refresh } = await useAsyncData(
  'tenant-settings',
  () => api.get<{ items: Setting[] }>('/api/settings'),
)

const saving = ref<string | null>(null)

async function save(setting: Setting, value: unknown) {
  saving.value = setting.identifier

  try {
    await api.put(`/api/settings/${setting.identifier}`, { value })
    toast.success(t('settings.saved'))
    await refresh()
  } catch (error) {
    toast.error((error as Error).message)
  } finally {
    saving.value = null
  }
}
</script>

<template>
  <div class="flex flex-col gap-6">
    <SettingsTabs />

    <UiCard :title="t('settings.workspace')" :description="t('settings.hint')">
      <UiEmptyState v-if="!pending && (data?.items.length ?? 0) === 0" />

      <ul v-else class="flex flex-col divide-y divide-border">
        <li
          v-for="setting in data?.items ?? []"
          :key="setting.identifier"
          class="flex items-center justify-between gap-4 py-3"
        >
          <div class="min-w-0">
            <p class="text-sm text-fg">{{ setting.name }}</p>
            <p v-if="setting.description" class="text-xs text-fg-muted">{{ setting.description }}</p>
            <p class="mt-0.5 font-mono text-[11px] text-fg-muted">{{ setting.identifier }}</p>
          </div>

          <div class="flex shrink-0 items-center gap-2">
            <UiBadge v-if="setting.overridden" tone="brand">{{ t('settings.overridden') }}</UiBadge>

            <UiBadge v-if="!setting.tenantEditable" tone="neutral">
              {{ t('settings.platformManaged') }}
            </UiBadge>

            <input
              v-else-if="setting.type === 'bool'"
              type="checkbox"
              class="size-4"
              :checked="setting.effectiveValue === true"
              :disabled="saving === setting.identifier || !auth.can('settings.manage')"
              :aria-label="setting.name"
              @change="save(setting, ($event.target as HTMLInputElement).checked)"
            >

            <span v-else class="font-mono text-xs text-fg">{{ setting.effectiveValue }}</span>
          </div>
        </li>
      </ul>
    </UiCard>
  </div>
</template>
