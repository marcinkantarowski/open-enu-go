<script setup lang="ts">
import { injectionsFor } from '@ui-kit/composables/defineInjection'
import { useAuthStore } from '@ui-kit/stores/auth'

/**
 * The dashboard: a greeting, and whatever the modules put on it.
 *
 * It has no content of its own, on purpose. What belongs on the first screen
 * of a product is that product's business - a balance, the last messages, what
 * is due tomorrow - and each of those belongs to a module. A module adds a card
 * by registering a widget for `dashboard.widgets` in its `injections.ts`; this
 * page never learns which modules exist.
 */
const auth = useAuthStore()
const { t } = useI18n()

const empty = computed(() => injectionsFor('dashboard.widgets').length === 0)
</script>

<template>
  <div class="flex flex-col gap-6">
    <div>
      <h1 class="text-lg font-semibold text-fg">
        {{ t('app.dashboard.greeting', { name: auth.user?.displayName ?? auth.user?.email ?? '' }) }}
      </h1>
      <p class="text-sm text-fg-muted">
        {{ t('app.dashboard.workspace') }}
        <span class="font-medium text-fg" data-testid="current-tenant">{{ auth.tenantName ?? auth.tenantId ?? '-' }}</span>
      </p>
    </div>

    <!-- Any module may add a card here without this file knowing it exists.
         Two columns from `lg` up; a widget that wants the full width says so
         itself with `lg:col-span-2`. -->
    <div class="grid items-start gap-6 lg:grid-cols-2">
      <InjectionPoint name="dashboard.widgets" />
    </div>

    <UiCard v-if="empty">
      <UiEmptyState :title="t('app.dashboard.empty')" :description="t('app.dashboard.emptyHint')" />
    </UiCard>
  </div>
</template>
