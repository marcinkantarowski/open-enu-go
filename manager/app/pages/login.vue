<script setup lang="ts">
import { ApiError } from '@ui-kit/composables/useApi'

definePageMeta({ layout: 'auth', public: true })

const { login } = useOperator()
const { t } = useI18n()

const email = ref('')
const password = ref('')
const error = ref<string | null>(null)
const busy = ref(false)

async function submit() {
  busy.value = true
  error.value = null

  try {
    await login(email.value, password.value)
    await navigateTo('/')
  } catch (e) {
    // 429 has its own message: at five attempts per fifteen minutes, an operator
    // locked out needs to know it is the limiter and not their password.
    error.value = e instanceof ApiError && e.status === 429
      ? t('manager.rateLimited')
      : e instanceof ApiError && e.status === 401
        ? t('auth.invalidCredentials')
        : (e as Error).message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div>
    <AuthHeading :title="t('manager.loginTitle')" :subtitle="t('manager.loginSubtitle')" />

    <form class="flex flex-col gap-5" data-testid="login-form" @submit.prevent="submit">
      <UiField v-slot="field" :label="t('auth.email')">
        <UiInput
          :id="field.id"
          v-model="email"
          type="email"
          autocomplete="username"
          data-testid="login-email"
          :described-by="field.describedBy"
        />
      </UiField>

      <UiField v-slot="field" :label="t('auth.password')">
        <UiInput
          :id="field.id"
          v-model="password"
          type="password"
          autocomplete="current-password"
          data-testid="login-password"
          :described-by="field.describedBy"
        />
      </UiField>

      <UiAlert v-if="error" tone="danger" data-testid="login-error">{{ error }}</UiAlert>

      <UiButton type="submit" variant="primary" size="lg" :loading="busy" data-testid="login-submit">
        {{ busy ? t('auth.signingIn') : t('auth.signIn') }}
      </UiButton>
    </form>
  </div>
</template>
