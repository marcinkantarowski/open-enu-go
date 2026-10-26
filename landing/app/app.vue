<script setup lang="ts">
const { public: cfg } = useRuntimeConfig()
const { locale } = useI18n()

useHead({
  htmlAttrs: { lang: locale },
  titleTemplate: title => (title && title !== cfg.appName ? `${title} · ${cfg.appName}` : cfg.appName as string),
})
</script>

<template>
  <NuxtLayout>
    <NuxtPage />
  </NuxtLayout>
</template>
