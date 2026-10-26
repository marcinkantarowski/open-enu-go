<script setup lang="ts">
import { ApiError } from '@ui-kit/composables/useApi'

/**
 * THE SCREEN TO COPY.
 *
 * It exists to prove, in a browser and against the real API, the five things
 * that are easy to claim and easy to get wrong:
 *
 *   1. **Optimistic locking.** A rename sends `If-Match` with the version the
 *      row was read at; a stale one comes back 409 with both versions and the
 *      saved record, and the conflict bar offers a real choice.
 *   2. **Realtime.** A create is broadcast on this tenant's Mercure topic, so a
 *      second tab updates without polling - and another tenant's stream receives
 *      nothing, which is asserted in `e2e/tests/example.spec.ts`.
 *   3. **Long jobs.** Archive returns a job id before any work has happened; the
 *      progress bar follows it to completion.
 *   4. **Feature flags.** Archive is behind one. Turned off it is a 404, which
 *      this page has to present as "unavailable" rather than as an error.
 *   5. **Tenant scoping.** Everything here is scoped by the token, so the list
 *      is empty - never another tenant's - when no tenant is in context.
 */
definePageMeta({ middleware: 'permission', permission: 'example.view' })

interface Project {
  id: string
  name: string
  attributes: Record<string, unknown>
  attachmentId: string | null
  status: 'active' | 'archived'
  version: number
  createdAt: string
}

const api = useApi()
const auth = useAuthStore()
const toast = useToast()
const { t } = useI18n()
const { conflict, resolving, attempt, reload, overwrite, dismiss } = useConflict()

const { data, pending, refresh } = await useAsyncData(
  'example-projects',
  () => api.get<{ items: Project[], meta: { total: number } }>('/api/projects'),
)

// ── create ────────────────────────────────────────────────────────────────
const creating = ref(false)
const draft = reactive({ name: '', clientReference: '' })
const saving = ref(false)

async function create() {
  saving.value = true

  try {
    await api.post('/api/projects', { ...draft })
    creating.value = false
    draft.name = ''
    draft.clientReference = ''
    // No refresh() here on purpose: the ProjectCreated broadcast arrives over
    // the open stream and refreshes the list, which is the behaviour being
    // demonstrated. A refresh here would hide a broken one.
  } catch (error) {
    toast.error((error as Error).message)
  } finally {
    saving.value = false
  }
}

// ── rename, with the conflict path ────────────────────────────────────────
const editing = ref<Project | null>(null)
const draftName = ref('')

function startEdit(project: Project) {
  editing.value = project
  draftName.value = project.name
  dismiss()
}

async function save() {
  const project = editing.value
  if (!project) return

  // `attempt` captures a 409 instead of throwing it; anything else still
  // throws, because a 500 is not something the user can resolve by choosing.
  const result = await attempt(() =>
    api.patch<Project>(`/api/projects/${project.id}`, { name: draftName.value }, { ifMatch: project.version }),
  )

  if (result === null) return

  editing.value = null
  toast.success(t('example.renamed'))
  await refresh()
}

async function takeTheirs() {
  await reload(async () => {
    await refresh()
    editing.value = null
  })
}

async function keepMine() {
  const project = editing.value
  if (!project) return

  await overwrite(version =>
    api.patch<Project>(`/api/projects/${project.id}`, { name: draftName.value }, { ifMatch: version }),
  )

  editing.value = null
  await refresh()
}

// ── the live stream ───────────────────────────────────────────────────────
// Scoped to this tenant's topic by the subscriber token, so another tenant's
// create cannot arrive here even if this page asked for it.
//
// The feed below the list shows the raw events as they arrive. It is here for
// one reason: it is the only place where NOT receiving something is visible,
// which is what a browser test of tenant isolation has to look at.
const feed = ref<Array<{ event: string, at: string, subject: string }>>([])

useAppEvent('example.project.created', (message) => {
  feed.value = [{ event: message.event, at: message.occurredAt, subject: message.subjectId }, ...feed.value].slice(0, 5)
  void refresh()
})

// ── the long job, behind a flag ───────────────────────────────────────────
const jobId = ref<string | null>(null)
const archiving = ref(false)

async function archive() {
  archiving.value = true

  try {
    const { jobId: id } = await api.post<{ jobId: string }>('/api/projects/archive')
    jobId.value = id
  } catch (error) {
    // A disabled flag is a 404 by design - "not built yet" and "switched off
    // during an incident" look identical from outside - so this page must not
    // report it as a missing page.
    toast.error(
      error instanceof ApiError && error.status === 404
        ? t('example.archiveUnavailable')
        : (error as Error).message,
    )
  } finally {
    archiving.value = false
  }
}

async function onArchived(result: Record<string, unknown>) {
  jobId.value = null
  toast.success(t('example.archived', { count: Number(result.archived ?? 0) }))
  await refresh()
}
</script>

<template>
  <div class="flex flex-col gap-6">
    <div class="flex items-start justify-between gap-4">
      <div>
        <h1 class="text-lg font-semibold text-fg">{{ t('example.title') }}</h1>
        <p class="text-sm text-fg-muted">{{ t('example.subtitle') }}</p>
      </div>

      <div class="flex items-center gap-3">
        <RealtimeIndicator />
        <UiButton
          v-if="auth.can('example.manage')"
          size="sm"
          :loading="archiving"
          data-testid="example-archive"
          @click="archive"
        >
          {{ t('example.archive') }}
        </UiButton>
        <UiButton
          v-if="auth.can('example.manage')"
          variant="primary"
          size="sm"
          data-testid="example-new"
          @click="creating = true"
        >
          {{ t('example.create') }}
        </UiButton>
      </div>
    </div>

    <UiCard v-if="jobId">
      <JobProgress :job-id="jobId" @done="onArchived" @failed="toast.error($event)" />
    </UiCard>

    <UiCard>
      <UiTable
        :columns="[
          { key: 'name', label: t('example.name') },
          { key: 'status', label: t('example.status') },
          { key: 'version', label: t('example.version'), numeric: true },
          { key: 'createdAt', label: t('example.created') },
          { key: 'actions', label: t('common.actions') },
        ]"
        :rows="data?.items ?? []"
        :loading="pending"
        :empty-title="t('example.empty')"
      >
        <template #cell-name="{ row }">
          <span data-testid="example-item-name">{{ row.name }}</span>
        </template>

        <template #cell-status="{ row }">
          <UiBadge :tone="row.status === 'archived' ? 'neutral' : 'success'">
            {{ t(`example.status.${row.status}`) }}
          </UiBadge>
        </template>

        <template #cell-actions="{ row }">
          <UiButton
            v-if="auth.can('example.manage') && row.status !== 'archived'"
            variant="ghost"
            size="sm"
            data-testid="example-rename"
            @click="startEdit(row)"
          >
            {{ t('example.rename') }}
          </UiButton>
        </template>
      </UiTable>
    </UiCard>

    <!-- Other modules may hang widgets here without this file knowing them. -->
    <InjectionPoint name="example.list.footer" />

    <UiModal v-model:open="creating" :title="t('example.create')">
      <div class="flex flex-col gap-4">
        <UiField v-slot="field" :label="t('example.name')" required>
          <UiInput :id="field.id" v-model="draft.name" data-testid="example-name" :described-by="field.describedBy" />
        </UiField>

        <UiField v-slot="field" :label="t('example.clientReference')" :hint="t('example.clientReferenceHint')">
          <UiInput :id="field.id" v-model="draft.clientReference" :described-by="field.describedBy" />
        </UiField>
      </div>

      <template #footer>
        <UiButton variant="ghost" @click="creating = false">{{ t('common.cancel') }}</UiButton>
        <UiButton
          variant="primary"
          :loading="saving"
          :disabled="!draft.name"
          data-testid="example-create-save"
          @click="create"
        >
          {{ t('common.save') }}
        </UiButton>
      </template>
    </UiModal>

    <UiModal :open="editing !== null" :title="t('example.rename')" @update:open="editing = null">
      <div class="flex flex-col gap-4">
        <!-- Inside the dialog, not behind it: `showModal()` makes the rest of
             the page inert, so a conflict bar on the page would be visible and
             unclickable. It also belongs here because this is where the edit
             they are about to lose is. -->
        <ConflictBar
          :conflict="conflict"
          :busy="resolving"
          @reload="takeTheirs"
          @overwrite="keepMine"
          @dismiss="dismiss"
        />

        <UiField v-slot="field" :label="t('example.name')" :hint="t('example.versionHint', { version: editing?.version })">
          <UiInput
            :id="field.id"
            v-model="draftName"
            data-testid="example-rename-input"
            :described-by="field.describedBy"
          />
        </UiField>

        <!-- Attachments are owned by a row, so the upload carries the owner.
             The Attachment module never learns what a Project is. -->
        <UploadField
          v-if="editing && auth.can('attachment.manage')"
          :owner="{ type: 'example_project', id: editing.id }"
          @uploaded="toast.success(t('example.attached', { name: $event.filename }))"
        />
      </div>

      <template #footer>
        <UiButton variant="ghost" @click="editing = null">{{ t('common.cancel') }}</UiButton>
        <UiButton variant="primary" data-testid="example-rename-save" @click="save">{{ t('common.save') }}</UiButton>
      </template>
    </UiModal>
    <UiCard v-if="feed.length > 0" :title="t('example.live')" :description="t('example.liveHint')">
      <ul class="flex flex-col gap-1 text-xs" data-testid="live-feed">
        <li v-for="(entry, index) in feed" :key="`${entry.at}-${index}`" class="font-mono text-fg">
          {{ entry.at }} · {{ entry.event }} · {{ entry.subject }}
        </li>
      </ul>
    </UiCard>
  </div>
</template>
