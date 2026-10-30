<script setup lang="ts">
import type { NuxtError } from '#app'
import { ApiError } from '@ui-kit/composables/useApi'

/**
 * What a failed navigation looks like - in the app's words and language,
 * not Nuxt's default English page. The permission middleware aborts with a
 * 403, an unknown route is a 404, anything else is a failure with the
 * request id when there is one, so it can be quoted to support.
 */
const props = defineProps<{ error: NuxtError }>()
const { t } = useI18n()
const { public: cfg } = useRuntimeConfig()

const code = computed(() => props.error.statusCode)
const kind = computed(() => code.value === 403 ? 'forbidden' : code.value === 404 ? 'notFound' : 'failed')
const requestId = computed(() => {
  const cause = props.error.cause
  if (cause instanceof ApiError && cause.requestId) return cause.requestId
  const data = props.error.data as { requestId?: string } | undefined
  return data?.requestId
})

useHead({ title: () => t(`app.error.${kind.value}.title`) })

// clearError, not a link: the error state belongs to the app and stays
// on screen until it is cleared.
const home = () => clearError({ redirect: '/' })
</script>

<template>
  <main class="flex min-h-screen items-center justify-center bg-surface px-6 py-12">
    <div class="flex w-full max-w-md flex-col gap-6" data-testid="error-page">
      <BrandMark :name="String(cfg.appName)" />
      <div>
        <p class="text-sm font-medium text-fg-muted">{{ code }}</p>
        <h1 class="mt-1 text-2xl font-semibold tracking-tight text-fg">{{ t(`app.error.${kind}.title`) }}</h1>
        <p class="mt-2 text-sm leading-relaxed text-fg-muted">{{ t(`app.error.${kind}.description`) }}</p>
        <p v-if="requestId" class="mt-2 font-mono text-xs text-fg-muted" data-testid="error-request-id">{{ t('common.requestId', { id: requestId }) }}</p>
      </div>
      <div>
        <UiButton variant="primary" data-testid="error-home" @click="home">{{ t('app.error.home') }}</UiButton>
      </div>
    </div>
  </main>
</template>
