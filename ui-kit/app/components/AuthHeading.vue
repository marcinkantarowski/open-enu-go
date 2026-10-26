<script setup lang="ts">
/** What a sign-in screen is for, said once above its form. */
defineProps<{ title: string, subtitle?: string }>()
</script>

<template>
  <div class="mb-6">
    <h1 class="text-2xl font-semibold tracking-tight text-fg">{{ title }}</h1>
    <p v-if="subtitle" class="mt-2 text-sm leading-relaxed text-fg-muted">{{ subtitle }}</p>
  </div>
</template>
