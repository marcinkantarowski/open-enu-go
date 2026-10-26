<script setup lang="ts">
const { public: cfg } = useRuntimeConfig()
const { t } = useI18n()
</script>

<template>
  <!-- Two halves from `lg` up: what this is on the left, what to do on the right.
       Below that the left half is decoration and goes, so a phone gets the form. -->
  <div class="grid min-h-screen lg:grid-cols-[minmax(0,5fr)_minmax(0,6fr)]">
    <aside class="relative hidden overflow-hidden bg-brand-800 lg:flex lg:flex-col lg:justify-between lg:p-12">
      <div
        class="pointer-events-none absolute inset-0 bg-linear-to-br from-brand-600 via-brand-700 to-brand-900"
        aria-hidden="true"
      />
      <div class="pointer-events-none absolute -right-24 -top-24 size-96 rounded-full bg-white/10 blur-3xl" aria-hidden="true" />
      <div class="pointer-events-none absolute -bottom-32 -left-16 size-112 rounded-full bg-brand-400/20 blur-3xl" aria-hidden="true" />

      <BrandMark class="relative" :name="String(cfg.appName)" tone="inverse" />

      <div class="relative max-w-md">
        <p class="text-3xl font-semibold leading-tight tracking-tight text-white">{{ t('app.auth.headline') }}</p>
        <p class="mt-4 text-sm leading-relaxed text-white/75">{{ t('app.auth.tagline') }}</p>
      </div>

      <p class="relative text-xs text-white/50">{{ cfg.domain }}</p>
    </aside>

    <main class="flex items-center justify-center bg-surface px-6 py-12">
      <div class="w-full max-w-sm">
        <BrandMark class="mb-10 lg:hidden" :name="String(cfg.appName)" />
        <slot />
      </div>
    </main>
  </div>
</template>
