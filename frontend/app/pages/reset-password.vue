<script setup lang="ts">
definePageMeta({ layout: 'auth', public: true, middleware: 'guest' })

const api = useApi()
const route = useRoute()
const { t } = useI18n()

// From the emailed link. Single-use and expiring - the server enforces both.
const token = computed(() => (typeof route.query.token === 'string' ? route.query.token : ''))

const password = ref('')
const error = ref<string | null>(null)
const busy = ref(false)

async function submit() {
  busy.value = true
  error.value = null

  try {
    await api.post('/api/auth/reset-password', { token: token.value, password: password.value })
    await navigateTo('/login')
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div>
    <AuthHeading :title="t('app.reset.title')" :subtitle="t('app.reset.subtitle')" />

    <form class="flex flex-col gap-5" @submit.prevent="submit">
      <UiAlert v-if="!token" tone="warning">{{ t('app.reset.missingToken') }}</UiAlert>

      <UiField v-slot="field" :label="t('app.reset.newPassword')" :hint="t('app.register.passwordHint')">
        <UiInput
          :id="field.id"
          v-model="password"
          type="password"
          autocomplete="new-password"
          :described-by="field.describedBy"
        />
      </UiField>

      <UiAlert v-if="error" tone="danger">{{ error }}</UiAlert>

      <UiButton type="submit" variant="primary" size="lg" :loading="busy" :disabled="!token">
        {{ t('app.reset.submit') }}
      </UiButton>
    </form>

    <p class="mt-8 text-center text-sm">
      <NuxtLink to="/login" class="font-medium text-brand-700 hover:underline">{{ t('app.auth.backToLogin') }}</NuxtLink>
    </p>
  </div>
</template>
