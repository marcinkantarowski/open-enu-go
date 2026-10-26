<script setup lang="ts">
import { useAuthStore } from '@ui-kit/stores/auth'

/**
 * Moving between the workspaces this person belongs to.
 *
 * Switching re-issues the session rather than setting a variable: the tenant
 * lives in the token (ADR-0010), so a switch is a new token and a fresh scope
 * for every request after it. That is why this reloads the page - every cached
 * list on screen belongs to the workspace being left.
 *
 * Renders nothing for the overwhelming majority who belong to one workspace.
 */
const auth = useAuthStore()
const { switchTenant } = useAuth()
const { t } = useI18n()
const toast = useToast()

const busy = ref(false)

const options = computed(() =>
  auth.memberships
    .filter(m => m.status === 'active')
    .map(m => ({ value: m.tenantId, label: m.tenantName ?? m.tenantId })),
)

const selected = computed({
  get: () => auth.tenantId ?? '',
  set: (tenantId: string) => { void change(tenantId) },
})

async function change(tenantId: string) {
  if (!tenantId || tenantId === auth.tenantId || busy.value) return

  busy.value = true

  try {
    await switchTenant(tenantId)
    // A full reload, not a route change. Anything already fetched belongs to
    // the workspace being left, and a targeted invalidation would have to know
    // about every module's state - which is exactly the coupling the module
    // boundaries exist to prevent.
    window.location.assign('/')
  } catch (error) {
    toast.error((error as Error).message)
    busy.value = false
  }
}
</script>

<template>
  <UiSelect
    v-if="auth.canSwitchTenant"
    v-model="selected"
    :options="options"
    :disabled="busy"
    :aria-label="t('tenant.switch')"
    class="max-w-48"
    data-testid="tenant-switcher"
  />
</template>
