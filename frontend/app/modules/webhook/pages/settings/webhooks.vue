<script setup lang="ts">
definePageMeta({ middleware: 'permission', permission: 'webhook.view' })

/**
 * Where a tenant's events go, and what happened when they went.
 *
 * The delivery log is the substance of this page rather than a debugging aid:
 * "did you send it?" is the only question anyone asks about webhooks, and a
 * settings screen that lists endpoints without answering it is a form.
 */
interface Endpoint {
  id: string
  url: string
  events: string[]
  active: boolean
  createdAt: string
}

interface Delivery {
  id: string
  endpointId: string
  event: string
  status: 'pending' | 'delivered' | 'failed'
  attempts: number
  responseCode: number | null
  responseBody: string | null
  lastAttemptAt: string | null
  createdAt: string
}

const api = useApi()
const auth = useAuthStore()
const toast = useToast()
const { t } = useI18n()

const { data: endpoints, refresh: refreshEndpoints } = await useAsyncData(
  'webhook-endpoints',
  () => api.get<{ items: Endpoint[] }>('/api/webhooks'),
)

const { data: deliveries, refresh: refreshDeliveries } = await useAsyncData(
  'webhook-deliveries',
  () => api.get<{ items: Delivery[] }>('/api/webhooks/deliveries'),
)

// The event names a tenant may subscribe to. Hard-coded here and nowhere else
// on purpose: a `GET /api/events` listing every event the system can emit is a
// map of the application's internals, handed to anyone with a login.
const available = ['example.project.created']

const creating = ref(false)
const busy = ref(false)
const form = reactive({ url: '', events: [] as string[] })
const secret = ref<string | null>(null)

async function create() {
  busy.value = true

  try {
    const created = await api.post<Endpoint & { secret: string }>('/api/webhooks', { ...form })
    secret.value = created.secret
    creating.value = false
    form.url = ''
    form.events = []
    await refreshEndpoints()
  } catch (error) {
    toast.error((error as Error).message)
  } finally {
    busy.value = false
  }
}

async function deactivate(endpoint: Endpoint) {
  if (!confirm(t('webhook.deactivateConfirm', { url: endpoint.url }))) return

  try {
    await api.del(`/api/webhooks/${endpoint.id}`)
    toast.success(t('webhook.deactivated'))
    await refreshEndpoints()
  } catch (error) {
    toast.error((error as Error).message)
  }
}

async function redeliver(delivery: Delivery) {
  try {
    await api.post(`/api/webhooks/deliveries/${delivery.id}/redeliver`)
    toast.success(t('webhook.redelivering'))
    await refreshDeliveries()
  } catch (error) {
    toast.error((error as Error).message)
  }
}

const tones: Record<Delivery['status'], 'success' | 'warning' | 'danger'> = {
  delivered: 'success',
  pending: 'warning',
  failed: 'danger',
}

async function copySecret() {
  if (secret.value) await navigator.clipboard.writeText(secret.value)
  toast.success(t('common.copied'))
}
</script>

<template>
  <div class="flex flex-col gap-6">
    <SettingsTabs />

    <UiCard :title="t('webhook.endpoints')" :description="t('webhook.endpointsHint')">
      <template #header>
        <UiButton
          v-if="auth.can('webhook.manage')"
          variant="primary"
          size="sm"
          data-testid="webhook-new"
          @click="creating = true"
        >
          {{ t('webhook.add') }}
        </UiButton>
      </template>

      <UiTable
        :columns="[
          { key: 'url', label: t('webhook.url') },
          { key: 'events', label: t('webhook.events') },
          { key: 'active', label: t('webhook.state') },
          { key: 'actions', label: '' },
        ]"
        :rows="endpoints?.items ?? []"
        :empty-title="t('webhook.noEndpoints')"
      >
        <template #cell-url="{ row }">
          <span class="font-mono text-xs" data-testid="webhook-url">{{ row.url }}</span>
        </template>

        <template #cell-events="{ row }">
          <span class="font-mono text-xs text-fg-muted">{{ row.events.join(', ') }}</span>
        </template>

        <template #cell-active="{ row }">
          <UiBadge :tone="row.active ? 'success' : 'neutral'">
            {{ row.active ? t('webhook.active') : t('webhook.inactive') }}
          </UiBadge>
        </template>

        <template #cell-actions="{ row }">
          <UiButton
            v-if="row.active && auth.can('webhook.manage')"
            variant="ghost"
            size="sm"
            @click="deactivate(row)"
          >
            {{ t('webhook.deactivate') }}
          </UiButton>
        </template>
      </UiTable>
    </UiCard>

    <UiCard :title="t('webhook.deliveries')" :description="t('webhook.deliveriesHint')">
      <template #header>
        <UiButton size="sm" @click="refreshDeliveries()">{{ t('common.retry') }}</UiButton>
      </template>

      <UiTable
        :columns="[
          { key: 'createdAt', label: t('webhook.when') },
          { key: 'event', label: t('webhook.event') },
          { key: 'status', label: t('webhook.state') },
          { key: 'attempts', label: t('webhook.attempts'), numeric: true },
          { key: 'responseCode', label: t('webhook.response'), numeric: true },
          { key: 'actions', label: '' },
        ]"
        :rows="deliveries?.items ?? []"
        :empty-title="t('webhook.noDeliveries')"
      >
        <template #cell-event="{ row }">
          <span class="font-mono text-xs">{{ row.event }}</span>
        </template>

        <template #cell-status="{ row }">
          <UiBadge :tone="tones[row.status]" data-testid="delivery-status">{{ t(`webhook.status.${row.status}`) }}</UiBadge>
        </template>

        <template #cell-responseCode="{ row }">
          <span class="font-mono text-xs">{{ row.responseCode ?? '-' }}</span>
        </template>

        <template #cell-actions="{ row }">
          <UiButton
            v-if="auth.can('webhook.manage')"
            variant="ghost"
            size="sm"
            data-testid="webhook-redeliver"
            @click="redeliver(row)"
          >
            {{ t('webhook.redeliver') }}
          </UiButton>
        </template>
      </UiTable>
    </UiCard>

    <UiModal v-model:open="creating" :title="t('webhook.add')">
      <div class="flex flex-col gap-4">
        <UiField v-slot="field" :label="t('webhook.url')" :hint="t('webhook.urlHint')" required>
          <UiInput
            :id="field.id"
            v-model="form.url"
            placeholder="https://"
            data-testid="webhook-url-input"
            :described-by="field.describedBy"
          />
        </UiField>

        <UiField :label="t('webhook.events')" :hint="t('webhook.eventsHint')">
          <div class="flex flex-col gap-1 rounded-lg border border-border p-2">
            <label v-for="event in available" :key="event" class="flex items-center gap-2 text-sm">
              <input v-model="form.events" type="checkbox" :value="event" class="size-4">
              <span class="font-mono text-xs">{{ event }}</span>
            </label>
          </div>
        </UiField>
      </div>

      <template #footer>
        <UiButton variant="ghost" @click="creating = false">{{ t('common.cancel') }}</UiButton>
        <UiButton
          variant="primary"
          :loading="busy"
          :disabled="!form.url || form.events.length === 0"
          data-testid="webhook-save"
          @click="create"
        >
          {{ t('common.save') }}
        </UiButton>
      </template>
    </UiModal>

    <UiModal :open="secret !== null" :title="t('webhook.secretTitle')" @update:open="secret = null">
      <div class="flex flex-col gap-3">
        <UiAlert tone="warning">{{ t('webhook.secretWarning') }}</UiAlert>
        <code class="block break-all rounded-lg bg-surface-sunken p-3 font-mono text-xs" data-testid="webhook-secret">
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
