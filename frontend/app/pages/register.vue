<script setup lang="ts">
definePageMeta({ layout: 'auth', public: true, middleware: 'guest' })

const api = useApi()
const { t, locale } = useI18n()

const form = reactive({ tenantName: '', email: '', password: '', displayName: '' })
const error = ref<string | null>(null)
const busy = ref(false)
const sent = ref(false)

async function submit() {
  busy.value = true
  error.value = null

  try {
    await api.post('/api/auth/register', { ...form, locale: locale.value })
    // 202, not a session: the account is unusable until the address is proved,
    // so there is deliberately nothing to sign into yet.
    sent.value = true
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div v-if="sent">
    <AuthHeading
      :title="t('app.register.checkEmailTitle')"
      :subtitle="t('app.register.checkEmailBody', { email: form.email })"
    />

    <NuxtLink to="/login" class="text-sm font-medium text-brand-700 hover:underline">
      {{ t('app.auth.backToLogin') }}
    </NuxtLink>
  </div>

  <div v-else>
    <AuthHeading :title="t('app.register.title')" :subtitle="t('app.register.subtitle')" />

    <form class="flex flex-col gap-5" @submit.prevent="submit">
      <UiField v-slot="field" :label="t('app.register.workspace')" required>
        <UiInput :id="field.id" v-model="form.tenantName" :described-by="field.describedBy" />
      </UiField>

      <UiField v-slot="field" :label="t('app.register.yourName')">
        <UiInput :id="field.id" v-model="form.displayName" autocomplete="name" :described-by="field.describedBy" />
      </UiField>

      <UiField v-slot="field" :label="t('auth.email')" required>
        <UiInput :id="field.id" v-model="form.email" type="email" autocomplete="username" :described-by="field.describedBy" />
      </UiField>

      <UiField
        v-slot="field"
        :label="t('auth.password')"
        :hint="t('app.register.passwordHint')"
        required
      >
        <UiInput
          :id="field.id"
          v-model="form.password"
          type="password"
          autocomplete="new-password"
          :described-by="field.describedBy"
        />
      </UiField>

      <UiAlert v-if="error" tone="danger">{{ error }}</UiAlert>

      <UiButton type="submit" variant="primary" size="lg" :loading="busy">{{ t('app.register.submit') }}</UiButton>
    </form>

    <p class="mt-8 text-center text-sm text-fg-muted">
      {{ t('app.register.haveAccount') }}
      <NuxtLink to="/login" class="font-medium text-brand-700 hover:underline">{{ t('auth.signIn') }}</NuxtLink>
    </p>
  </div>
</template>
