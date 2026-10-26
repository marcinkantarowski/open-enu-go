<script setup lang="ts">
/**
 * The unread count, and a way to the feed.
 *
 * Polled rather than streamed. A notification is not urgent by definition - if
 * it were, it would be an email - and a minute of latency costs nothing next to
 * a second open connection per tab.
 */
const { unread, load } = useNotifications()
const { t } = useI18n()

let timer: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  void load()
  timer = setInterval(() => void load(), 60_000)
})

onBeforeUnmount(() => {
  if (timer !== null) clearInterval(timer)
})
</script>

<template>
  <NuxtLink
    to="/notifications"
    class="relative rounded-md px-2 py-1 text-fg-muted hover:bg-surface-sunken hover:text-fg"
    :aria-label="t('notification.title')"
    data-testid="notification-bell"
  >
    🔔
    <span
      v-if="unread > 0"
      class="absolute -right-1 -top-1 min-w-4 rounded-full bg-danger px-1 text-center text-[10px] font-semibold text-white"
      data-testid="notification-unread"
    >{{ unread }}</span>
  </NuxtLink>
</template>
