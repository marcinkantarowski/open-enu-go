<script setup lang="ts">
import type { NavigationItem } from '@ui-kit/composables/defineNavigation'
import { useAuthStore } from '@ui-kit/stores/auth'

const auth = useAuthStore()
const { logout } = useOperator()
const { t } = useI18n()

// The console's own four screens, then whatever the modules add. Found by glob
// for the reason the tenant app's menu is: a central list is the file two
// branches both edit and the one somebody forgets. No permission filter - an
// operator has a role, not a permission list, and the realm is the gate.
const moduleNavigation = import.meta.glob<{ default: NavigationItem[] }>(
  '../modules/*/navigation.ts',
  { eager: true },
)

const links = computed(() => [
  { to: '/', label: t('manager.tenants'), order: 10 },
  { to: '/audit', label: t('manager.audit'), order: 20 },
  { to: '/flags', label: t('manager.flags'), order: 30 },
  { to: '/workers', label: t('manager.workers'), order: 40 },
  ...Object.values(moduleNavigation)
    .flatMap(mod => mod.default ?? [])
    .map(item => ({ to: item.to, label: t(item.label), order: item.order ?? 100 })),
].sort((a, b) => a.order - b.order))

function signOut() {
  logout()
  return navigateTo('/login')
}
</script>

<template>
  <div class="flex min-h-screen flex-col">
    <!-- Visually distinct from the tenant app on purpose. An operator with both
         open must never have to read the URL to know which one they are in. -->
    <header class="sticky top-0 z-10 flex h-16 items-center justify-between gap-4 bg-fg px-6 text-white">
      <div class="flex items-center gap-8">
        <NuxtLink to="/"><BrandMark :name="String($config.public.appName)" tone="inverse" /></NuxtLink>

        <nav class="flex gap-1">
          <NuxtLink
            v-for="link in links"
            :key="link.to"
            :to="link.to"
            class="rounded-lg px-3 py-1.5 text-sm text-white/70 transition-colors hover:bg-white/10 hover:text-white"
            exact-active-class="bg-white/15 font-medium text-white"
          >
            {{ link.label }}
          </NuxtLink>
        </nav>
      </div>

      <div class="flex items-center gap-4">
        <span class="text-sm text-white/70">{{ auth.user?.displayName ?? auth.user?.email }}</span>
        <button
          type="button"
          class="rounded-lg px-2.5 py-1 text-sm text-white/70 transition-colors hover:bg-white/10 hover:text-white"
          @click="signOut"
        >
          {{ t('auth.signOut') }}
        </button>
      </div>
    </header>

    <main class="mx-auto w-full max-w-7xl flex-1 px-6 py-8">
      <slot />
    </main>
  </div>
</template>
