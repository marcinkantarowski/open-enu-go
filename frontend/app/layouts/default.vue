<script setup lang="ts">
import { useAuthStore } from '@ui-kit/stores/auth'
import { useUiStore } from '@ui-kit/stores/ui'

const auth = useAuthStore()
const ui = useUiStore()
const { sections, topbar } = useNavigation()
const { logout } = useAuth()
const { t } = useI18n()
// One stream per tab, held for as long as the signed-in shell is mounted - so
// it survives moving between pages that do and do not listen. Widgets subscribe
// to it by event name; they do not each open a connection.
useMercure().hold()

// Initials stand in for a picture nobody uploaded: enough to tell at a glance
// whose session this tab holds.
const initials = computed(() => {
  const source = auth.user?.displayName || auth.user?.email || ''
  const parts = source.split(/[\s.@_-]+/).filter(Boolean)

  return ((parts[0]?.[0] ?? '') + (parts[1]?.[0] ?? '')).toUpperCase() || '?'
})

async function signOut() {
  await logout()
  await navigateTo('/login')
}
</script>

<template>
  <div class="min-h-screen">
    <ImpersonationBanner @exit="navigateTo('/login')" />

    <div class="flex min-h-screen">
      <aside
        v-show="ui.sidebarOpen"
        class="sticky top-0 flex h-screen w-64 shrink-0 flex-col border-r border-border bg-surface"
      >
        <NuxtLink to="/" class="flex h-16 items-center border-b border-border px-5">
          <BrandMark :name="String($config.public.appName)" />
        </NuxtLink>

        <nav class="flex flex-1 flex-col gap-5 overflow-y-auto px-3 py-4" :aria-label="t('app.nav.main')">
          <div v-for="section in sections" :key="section.group" class="flex flex-col gap-1">
            <p
              v-if="section.label"
              class="px-3 pb-1 text-xs font-semibold uppercase tracking-wide text-fg-muted"
              data-testid="nav-group"
            >
              {{ section.label }}
            </p>
            <NuxtLink
              v-for="item in section.items"
              :key="item.to"
              :to="item.to"
              class="rounded-lg px-3 py-2 text-sm transition-colors hover:bg-surface-sunken hover:text-fg"
              :class="section.label ? 'text-fg-muted' : 'font-medium text-fg'"
              exact-active-class="bg-brand-50 font-medium text-brand-700 hover:bg-brand-50 hover:text-brand-700"
            >
              {{ item.label }}
            </NuxtLink>
          </div>
        </nav>
      </aside>

      <div class="flex min-w-0 flex-1 flex-col">
        <header class="sticky top-0 z-10 flex h-16 items-center justify-between gap-4 border-b border-border bg-surface/90 px-6 backdrop-blur">
          <button
            type="button"
            class="rounded-lg p-2 text-fg-muted transition-colors hover:bg-surface-sunken hover:text-fg"
            :aria-label="t('app.nav.menu')"
            :aria-expanded="ui.sidebarOpen"
            @click="ui.toggleSidebar()"
          >
            <svg class="size-5" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" aria-hidden="true">
              <path d="M3.5 5.5h13M3.5 10h13M3.5 14.5h13" />
            </svg>
          </button>

          <div class="flex items-center gap-3 whitespace-nowrap">
            <RealtimeIndicator />
            <NotificationBell />
            <NuxtLink
              v-for="item in topbar"
              :key="item.to"
              :to="item.to"
              class="rounded-lg p-2 text-fg-muted transition-colors hover:bg-surface-sunken hover:text-fg"
              :aria-label="item.label"
              :title="item.label"
              data-testid="nav-topbar"
            >
              <svg class="size-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <circle cx="12" cy="12" r="3" />
                <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-2.82 1.17V21a2 2 0 1 1-4 0v-.09a1.65 1.65 0 0 0-2.82-1.17l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0-1.17-2.82H3a2 2 0 1 1 0-4h.09a1.65 1.65 0 0 0 1.17-2.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 2.82 1.17l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9c.26.6.85 1 1.51 1H21a2 2 0 1 1 0 4h-.09c-.66 0-1.25.4-1.51 1Z" />
              </svg>
            </NuxtLink>
            <TenantSwitcher />

            <span class="hidden h-6 w-px bg-border sm:block" aria-hidden="true" />

            <span class="flex items-center gap-2.5">
              <span
                class="inline-flex size-8 items-center justify-center rounded-full bg-brand-100 text-xs font-semibold text-brand-800"
                aria-hidden="true"
              >{{ initials }}</span>
              <span class="hidden text-sm font-medium text-fg sm:inline" data-testid="current-user">
                {{ auth.user?.displayName ?? auth.user?.email }}
              </span>
            </span>

            <UiButton size="sm" variant="ghost" data-testid="sign-out" @click="signOut">
              {{ t('auth.signOut') }}
            </UiButton>
          </div>
        </header>

        <main class="mx-auto w-full min-w-0 max-w-7xl flex-1 px-6 py-8">
          <slot />
        </main>
      </div>
    </div>
  </div>
</template>
