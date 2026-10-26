<script setup lang="ts">
definePageMeta({ middleware: 'permission', permission: 'notification.view' })

const { items, unread, load, markRead, markAllRead, argumentsFor } = useNotifications()
const { t } = useI18n()

await load()
</script>

<template>
  <div class="flex flex-col gap-6">
    <div class="flex items-start justify-between gap-4">
      <div>
        <h1 class="text-lg font-semibold text-fg">{{ t('notification.title') }}</h1>
        <p class="text-sm text-fg-muted">{{ t('notification.subtitle') }}</p>
      </div>

      <UiButton v-if="unread > 0" size="sm" data-testid="notification-read-all" @click="markAllRead">
        {{ t('notification.markAllRead') }}
      </UiButton>
    </div>

    <UiCard>
      <UiEmptyState
        v-if="items.length === 0"
        :title="t('notification.empty')"
        :description="t('notification.emptyHint')"
      />

      <ul v-else class="flex flex-col divide-y divide-border" data-testid="notification-list">
        <li
          v-for="item in items"
          :key="item.id"
          class="flex items-start justify-between gap-4 py-3"
          :class="item.read ? 'opacity-60' : ''"
        >
          <div class="min-w-0">
            <!-- Translated here, from the key and the arguments the server
                 stored. The same row reads correctly in either language. -->
            <p class="text-sm font-medium text-fg">{{ t(item.titleKey, argumentsFor(item)) }}</p>
            <p class="mt-0.5 text-xs text-fg-muted">{{ t(item.bodyKey, argumentsFor(item)) }}</p>
            <p class="mt-1 font-mono text-[11px] text-fg-muted">{{ item.createdAt }}</p>
          </div>

          <UiButton v-if="!item.read" variant="ghost" size="sm" @click="markRead(item.id)">
            {{ t('notification.markRead') }}
          </UiButton>
        </li>
      </ul>
    </UiCard>
  </div>
</template>
