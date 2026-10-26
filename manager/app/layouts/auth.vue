<script setup lang="ts">
const { public: cfg } = useRuntimeConfig()
const { t } = useI18n()
</script>

<template>
  <!-- Dark where the tenant app is light, on purpose: nobody should have to
       read the URL to know which of the two sign-in screens this is. -->
  <div class="relative flex min-h-screen items-center justify-center overflow-hidden bg-fg px-6 py-12">
    <div class="pointer-events-none absolute -top-40 left-1/2 size-144 -translate-x-1/2 rounded-full bg-brand-500/20 blur-3xl" aria-hidden="true" />

    <div class="relative w-full max-w-sm">
      <BrandMark class="mb-8" :name="String(cfg.appName)" tone="inverse" />

      <div class="rounded-card bg-surface p-8 shadow-raised">
        <slot />
      </div>

      <p class="mt-6 text-center text-xs leading-relaxed text-white/50">{{ t('manager.loginNotice') }}</p>
    </div>
  </div>
</template>
