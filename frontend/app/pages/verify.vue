<script setup lang="ts">
definePageMeta({ layout: 'auth', public: true })

const api = useApi()
const route = useRoute()
const { t } = useI18n()

const state = ref<'working' | 'done' | 'failed'>('working')
const message = ref('')

onMounted(async () => {
  const token = typeof route.query.token === 'string' ? route.query.token : ''

  if (!token) {
    state.value = 'failed'
    message.value = t('app.verify.missingToken')
    return
  }

  try {
    await api.post('/api/auth/verify', { token })
    state.value = 'done'
  } catch (e) {
    state.value = 'failed'
    // One message for expired, unknown and already-used: distinguishing them
    // would tell someone which links were real.
    message.value = (e as Error).message
  }
})
</script>

<template>
  <div>
    <div v-if="state === 'working'" class="flex items-center gap-3 text-sm text-fg-muted">
      <UiSpinner class="size-5 text-brand-600" />
      {{ t('app.verify.working') }}
    </div>

    <template v-else-if="state === 'done'">
      <AuthHeading :title="t('app.verify.done')" :subtitle="t('app.verify.doneHint')" />
      <UiButton variant="primary" size="lg" class="w-full" @click="navigateTo('/login')">{{ t('auth.signIn') }}</UiButton>
    </template>

    <template v-else>
      <AuthHeading :title="t('app.verify.failed')" />
      <UiAlert tone="danger">{{ message }}</UiAlert>
      <p class="mt-8 text-sm">
        <NuxtLink to="/login" class="font-medium text-brand-700 hover:underline">{{ t('app.auth.backToLogin') }}</NuxtLink>
      </p>
    </template>
  </div>
</template>
