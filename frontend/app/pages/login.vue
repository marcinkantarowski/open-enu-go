<script setup lang="ts">
import { ApiError } from '@ui-kit/composables/useApi'

definePageMeta({ layout: 'auth', public: true, middleware: 'guest' })

const { login } = useAuth()
const { t } = useI18n()
const route = useRoute()

const email = ref('')
const password = ref('')
const error = ref<string | null>(null)
const busy = ref(false)

async function submit() {
  busy.value = true
  error.value = null

  try {
    await login(email.value, password.value)
    // Back to wherever the guard interrupted, or the dashboard.
    await navigateTo(typeof route.query.next === 'string' ? route.query.next : '/')
  } catch (e) {
    // 401 is the deliberate single answer for wrong address AND wrong password;
    // the server will not say which, so neither does this.
    error.value = e instanceof ApiError && e.status === 401
      ? t('auth.invalidCredentials')
      : (e as Error).message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div>
    <AuthHeading :title="t('app.login.title')" :subtitle="t('app.login.subtitle')" />

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

      <div class="-mt-2 text-right text-sm">
        <NuxtLink to="/forgot-password" class="font-medium text-brand-700 hover:underline">
          {{ t('auth.forgotPassword') }}
        </NuxtLink>
      </div>

      <UiAlert v-if="error" tone="danger" data-testid="login-error">{{ error }}</UiAlert>

      <UiButton type="submit" variant="primary" size="lg" :loading="busy" data-testid="login-submit">
        {{ busy ? t('auth.signingIn') : t('auth.signIn') }}
      </UiButton>
    </form>

    <!-- Other ways in, from the modules that check them: single sign-on, a mailed link... -->
    <InjectionPoint name="auth.login.methods" />

    <p class="mt-8 text-center text-sm text-fg-muted">
      {{ t('app.login.noAccount') }}
      <NuxtLink to="/register" class="font-medium text-brand-700 hover:underline">{{ t('app.register') }}</NuxtLink>
    </p>
  </div>
</template>
