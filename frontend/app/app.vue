<script setup lang="ts">
const { public: cfg } = useRuntimeConfig()
const { locale } = useI18n()

useHead({
  title: cfg.appName as string,
  htmlAttrs: { lang: locale },
})
</script>

<template>
  <div>
    <NuxtLayout>
      <NuxtPage />
    </NuxtLayout>

    <!-- Mounted once, at the root: anything anywhere can raise a toast. -->
    <ToastHost />
  </div>
</template>
