<script setup lang="ts">
/**
 * One tenant, with the two things an operator actually comes here to do:
 * suspend it, and look at it through a member's eyes.
 */
interface Member {
  /** The USER's id - what the impersonation endpoint expects. */
  id: string
  email: string
  displayName: string | null
  role: string
  status: string
}

interface TenantDetail {
  id: string
  slug: string
  name: string
  status: string
  defaultLocale: string
  members: Member[]
}

const api = useApi()
const route = useRoute()
const toast = useToast()
const { t } = useI18n()
const { public: cfg } = useRuntimeConfig()

const id = computed(() => String(route.params.id))

const { data, pending, refresh } = await useAsyncData(
  () => `manager-tenant-${id.value}`,
  () => api.get<TenantDetail>(`/api/manager/tenants/${id.value}`),
)

const busy = ref(false)
const consultantEmail = ref('')
const assigning = ref(false)

async function assignConsultant() {
  assigning.value = true

  try {
    await api.post(`/api/manager/tenants/${id.value}/consultants`, { email: consultantEmail.value })
    toast.success(t('manager.consultantAssigned'))
    consultantEmail.value = ''
    await refresh()
  } catch (error) {
    toast.error((error as Error).message)
  } finally {
    assigning.value = false
  }
}

async function removeConsultant(member: Member) {
  if (!confirm(t('manager.consultantRemoveConfirm', { email: member.email }))) return

  try {
    await api.del(`/api/manager/tenants/${id.value}/consultants/${member.id}`)
    toast.success(t('manager.consultantRemoved'))
    await refresh()
  } catch (error) {
    toast.error((error as Error).message)
  }
}

async function suspend() {
  if (!confirm(t('manager.suspendConfirm', { name: data.value?.name ?? '' }))) return

  busy.value = true

  try {
    await api.post(`/api/manager/tenants/${id.value}/suspend`)
    toast.success(t('manager.suspended'))
    await refresh()
  } catch (error) {
    toast.error((error as Error).message)
  } finally {
    busy.value = false
  }
}

/**
 * Open the tenant app as this member.
 *
 * The session travels in the URL **fragment**, which is never sent to a server:
 * it stays out of access logs, out of `Referer` on the next navigation, and out
 * of anything sitting in front of the app. A query string would be in all three.
 *
 * The console does not render tenant screens itself - it hands the tenant app a
 * genuine `aud: app` token and gets out of the way.
 */
async function viewAs(member: Member) {
  if (!confirm(t('manager.impersonateConfirm', { email: member.email }))) return

  try {
    const session = await api.post<Record<string, unknown>>(
      `/api/manager/tenants/${id.value}/users/${member.id}/impersonate`,
    )

    const handoff = encodeURIComponent(JSON.stringify(session))
    window.open(`${String(cfg.appUrl)}/impersonate#${handoff}`, '_blank', 'noopener')
  } catch (error) {
    toast.error((error as Error).message)
  }
}
</script>

<template>
  <div class="flex flex-col gap-6">
    <div class="flex items-start justify-between gap-4">
      <div>
        <NuxtLink to="/" class="text-xs text-fg-muted hover:underline">← {{ t('manager.tenants') }}</NuxtLink>
        <h1 class="text-lg font-semibold text-fg">{{ data?.name ?? '-' }}</h1>
        <p class="font-mono text-xs text-fg-muted">
          {{ data?.slug }} · <span data-testid="tenant-id">{{ data?.id }}</span>
        </p>
      </div>

      <UiButton
        v-if="data && data.status !== 'suspended'"
        variant="danger"
        :loading="busy"
        data-testid="tenant-suspend"
        @click="suspend"
      >
        {{ t('manager.suspend') }}
      </UiButton>
    </div>

    <UiCard :title="t('manager.members')" :description="t('manager.membersHint')">
      <UiTable
        :columns="[
          { key: 'email', label: t('auth.email') },
          { key: 'displayName', label: t('manager.name') },
          { key: 'role', label: t('manager.role') },
          { key: 'status', label: t('manager.status') },
          { key: 'actions', label: '' },
        ]"
        :rows="data?.members ?? []"
        :loading="pending"
        :empty-title="t('manager.noMembers')"
      >
        <template #cell-role="{ row }">
          <span data-testid="member-role">{{ t(`manager.roles.${row.role}`) }}</span>
        </template>

        <template #cell-actions="{ row }">
          <div class="flex justify-end gap-2">
            <UiButton
              v-if="row.role === 'consultant'"
              size="sm"
              variant="ghost"
              data-testid="consultant-remove"
              @click="removeConsultant(row)"
            >
              {{ t('manager.consultantRemove') }}
            </UiButton>
            <UiButton size="sm" data-testid="impersonate" @click="viewAs(row)">
              {{ t('manager.viewAs') }}
            </UiButton>
          </div>
        </template>
      </UiTable>
    </UiCard>

    <UiCard :title="t('manager.consultants')" :description="t('manager.consultantAssignHint')">
      <form class="flex flex-wrap items-end gap-3" @submit.prevent="assignConsultant">
        <UiField v-slot="field" :label="t('manager.consultantEmail')" class="min-w-64 flex-1">
          <UiInput
            :id="field.id"
            v-model="consultantEmail"
            type="email"
            required
            data-testid="consultant-email"
            :described-by="field.describedBy"
          />
        </UiField>
        <UiButton type="submit" variant="primary" :loading="assigning" data-testid="consultant-assign">
          {{ t('manager.consultantAssign') }}
        </UiButton>
      </form>
    </UiCard>

    <!-- What each module knows about this tenant - its balance, its limits, its
         pending requests - added by the module, without this page learning that
         the module exists. Rendered once the tenant is known to exist. -->
    <InjectionPoint v-if="data" name="manager.tenant" :context="{ tenantId: data.id }" />
  </div>
</template>
