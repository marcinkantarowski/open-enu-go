<script setup lang="ts">
definePageMeta({ middleware: 'permission', permission: 'api_key.view' })

/**
 * Machine credentials for this workspace.
 *
 * The secret is shown exactly once, at creation, and then never again - it is
 * hashed at rest, so "can you resend it?" has no answer but "make a new one".
 * The UI has to make that unmistakable at the moment it matters, which is what
 * the modal below is for.
 */
interface ApiKey {
  id: string
  name: string
  prefix: string
  permissions: string[]
  revoked: boolean
  expiresAt: string | null
  lastUsedAt: string | null
  createdAt: string
}

const api = useApi()
const auth = useAuthStore()
const toast = useToast()
const { t } = useI18n()

const { data, pending, refresh } = await useAsyncData(
  'api-keys',
  () => api.get<{ items: ApiKey[] }>('/api/api-keys'),
)

const creating = ref(false)
const busy = ref(false)
const form = reactive({ name: '', permissions: [] as string[], expiresInDays: 90 })
const secret = ref<string | null>(null)

/**
 * A key may only be granted permissions its creator already holds.
 *
 * `api_key.manage` is excluded on purpose and the server refuses it too: a
 * credential that can mint credentials removes the point of scoping them.
 */
const grantable = computed(() => auth.permissions.filter(p => p !== 'api_key.manage'))

async function create() {
  busy.value = true

  try {
    const created = await api.post<ApiKey & { secret: string }>('/api/api-keys', { ...form })
    secret.value = created.secret
    creating.value = false
    form.name = ''
    form.permissions = []
    await refresh()
  } catch (error) {
    toast.error((error as Error).message)
  } finally {
    busy.value = false
  }
}

async function revoke(key: ApiKey) {
  if (!confirm(t('apiKey.revokeConfirm', { name: key.name }))) return

  try {
    await api.del(`/api/api-keys/${key.id}`)
    toast.success(t('apiKey.revoked'))
    await refresh()
  } catch (error) {
    toast.error((error as Error).message)
  }
}

async function copySecret() {
  if (secret.value) await navigator.clipboard.writeText(secret.value)
  toast.success(t('common.copied'))
}
</script>

<template>
  <div class="flex flex-col gap-6">
    <SettingsTabs />

    <UiCard :title="t('apiKey.title')" :description="t('apiKey.hint')">
      <template #header>
        <UiButton
          v-if="auth.can('api_key.manage')"
          variant="primary"
          size="sm"
          data-testid="api-key-new"
          @click="creating = true"
        >
          {{ t('apiKey.create') }}
        </UiButton>
      </template>

      <UiTable
        :columns="[
          { key: 'name', label: t('apiKey.name') },
          { key: 'prefix', label: t('apiKey.prefix') },
          { key: 'permissions', label: t('apiKey.permissions') },
          { key: 'lastUsedAt', label: t('apiKey.lastUsed') },
          { key: 'actions', label: t('common.actions') },
        ]"
        :rows="data?.items ?? []"
        :loading="pending"
        :empty-title="t('apiKey.empty')"
      >
        <template #cell-permissions="{ row }">
          <span class="font-mono text-xs text-fg-muted">{{ (row.permissions as string[]).join(', ') }}</span>
        </template>

        <template #cell-actions="{ row }">
          <UiBadge v-if="row.revoked" tone="danger">{{ t('apiKey.revokedBadge') }}</UiBadge>
          <UiButton
            v-else-if="auth.can('api_key.manage')"
            variant="ghost"
            size="sm"
            @click="revoke(row)"
          >
            {{ t('apiKey.revoke') }}
          </UiButton>
        </template>
      </UiTable>
    </UiCard>

    <UiModal v-model:open="creating" :title="t('apiKey.create')">
      <div class="flex flex-col gap-4">
        <UiField v-slot="field" :label="t('apiKey.name')" required>
          <UiInput :id="field.id" v-model="form.name" :described-by="field.describedBy" />
        </UiField>

        <UiField :label="t('apiKey.permissions')" :hint="t('apiKey.permissionsHint')">
          <div class="flex max-h-48 flex-col gap-1 overflow-y-auto rounded-lg border border-border p-2">
            <label v-for="permission in grantable" :key="permission" class="flex items-center gap-2 text-sm">
              <input v-model="form.permissions" type="checkbox" :value="permission" class="size-4">
              <span class="font-mono text-xs">{{ permission }}</span>
            </label>
          </div>
        </UiField>

        <UiField v-slot="field" :label="t('apiKey.expiry')" :hint="t('apiKey.expiryHint')">
          <UiInput :id="field.id" v-model.number="form.expiresInDays" type="number" :described-by="field.describedBy" />
        </UiField>
      </div>

      <template #footer>
        <UiButton variant="ghost" @click="creating = false">{{ t('common.cancel') }}</UiButton>
        <UiButton
          variant="primary"
          :loading="busy"
          :disabled="!form.name || form.permissions.length === 0"
          @click="create"
        >
          {{ t('apiKey.create') }}
        </UiButton>
      </template>
    </UiModal>

    <UiModal :open="secret !== null" :title="t('apiKey.secretTitle')" @update:open="secret = null">
      <div class="flex flex-col gap-3">
        <UiAlert tone="warning">{{ t('apiKey.secretWarning') }}</UiAlert>
        <code class="block break-all rounded-lg bg-surface-sunken p-3 font-mono text-xs" data-testid="api-key-secret">
          {{ secret }}
        </code>
      </div>

      <template #footer>
        <UiButton @click="copySecret">{{ t('common.copy') }}</UiButton>
        <UiButton variant="primary" @click="secret = null">{{ t('common.close') }}</UiButton>
      </template>
    </UiModal>
  </div>
</template>
