<script setup lang="ts">
import { useAuthStore } from '../stores/auth'

/**
 * Always visible while an operator is viewing as someone else (ADR-0020).
 *
 * Deliberately loud and deliberately un-dismissible. Support access that the
 * person being supported cannot see is the thing that turns a useful tool into
 * a trust problem - and an operator who forgets which session they are in is how
 * a "quick fix" gets applied to the wrong account.
 *
 * There is no refresh cookie for an impersonated session, so leaving is simply
 * throwing the in-memory token away.
 */
const auth = useAuthStore()
const { t } = useI18n()

const emit = defineEmits<{ exit: [] }>()

function exit() {
  auth.clear()
  emit('exit')
}
</script>

<template>
  <div
    v-if="auth.impersonating"
    class="flex flex-wrap items-center justify-between gap-3 bg-warning px-4 py-2 text-sm text-black"
    role="alert"
    data-testid="impersonation-banner"
  >
    <div>
      <strong>{{ t('impersonation.title', { user: auth.user?.displayName ?? auth.user?.email ?? '-' }) }}</strong>
      <span class="ml-2 opacity-80">{{ t('impersonation.description') }}</span>
    </div>

    <button
      type="button"
      class="rounded-md bg-black/85 px-3 py-1 text-xs font-medium text-white hover:bg-black"
      @click="exit"
    >
      {{ t('impersonation.exit') }}
    </button>
  </div>
</template>
