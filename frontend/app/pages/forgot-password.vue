<script setup lang="ts">
definePageMeta({ layout: 'auth', public: true, middleware: 'guest' })

const api = useApi()
const { t } = useI18n()

const email = ref('')
const busy = ref(false)
const sent = ref(false)

async function submit() {
  busy.value = true

  try {
    await api.post('/api/auth/forgot-password', { email: email.value })
  } finally {
    // Shown whatever happened, including on failure. A form that behaves
    // differently for a known address turns password reset into a way to test
    // whether someone has an account here.
    sent.value = true
    busy.value = false
  }
}
</script>

<template>
  <div v-if="sent">
    <AuthHeading :title="t('app.forgot.sentTitle')" :subtitle="t('app.forgot.sent')" />

    <NuxtLink to="/login" class="text-sm font-medium text-brand-700 hover:underline">
      {{ t('app.auth.backToLogin') }}
    </NuxtLink>
  </div>

  <div v-else>
    <AuthHeading :title="t('app.forgot.title')" :subtitle="t('app.forgot.subtitle')" />

    <form class="flex flex-col gap-5" @submit.prevent="submit">
      <UiField v-slot="field" :label="t('auth.email')">
        <UiInput :id="field.id" v-model="email" type="email" autocomplete="username" :described-by="field.describedBy" />
      </UiField>

      <UiButton type="submit" variant="primary" size="lg" :loading="busy">{{ t('app.forgot.submit') }}</UiButton>
    </form>

    <p class="mt-8 text-center text-sm">
      <NuxtLink to="/login" class="font-medium text-brand-700 hover:underline">{{ t('app.auth.backToLogin') }}</NuxtLink>
    </p>
  </div>
</template>
