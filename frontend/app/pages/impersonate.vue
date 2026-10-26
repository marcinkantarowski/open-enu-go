<script setup lang="ts">
import { useAuthStore } from '@ui-kit/stores/auth'
import type { Session } from '@ui-kit/stores/auth'

/**
 * Where the operator console hands a support session to the tenant app.
 *
 * The session arrives in the URL **fragment**, never the query string. A
 * fragment is not sent to the server, so it stays out of access logs, out of
 * `Referer` headers on the next navigation, and out of anything sitting in
 * front of this app. The query string would be in all three.
 *
 * It is consumed and erased immediately: what is left in the address bar after
 * this page runs is `/impersonate`, with no credential in the history entry.
 */
definePageMeta({ layout: 'blank', public: true })

const auth = useAuthStore()
const { t } = useI18n()
const failed = ref(false)

onMounted(async () => {
  const raw = window.location.hash.replace(/^#/, '')

  try {
    const session = JSON.parse(atob(decodeURIComponent(raw))) as Session

    if (!session.token) throw new Error('No token in the handoff.')

    auth.setSession({ ...session, impersonating: true })

    // Replace, not push: the back button must not return to a URL that still
    // contains the token.
    history.replaceState(null, '', '/impersonate')
    await navigateTo('/', { replace: true })
  } catch {
    failed.value = true
  }
})
</script>

<template>
  <div class="flex min-h-screen items-center justify-center p-6">
    <UiAlert v-if="failed" tone="danger">{{ t('app.impersonate.failed') }}</UiAlert>
    <UiSpinner v-else class="size-6 text-brand-600" />
  </div>
</template>
