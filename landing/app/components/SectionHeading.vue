<script setup lang="ts">
withDefaults(defineProps<{ eyebrow?: string, title: string, lead?: string, level?: 'h1' | 'h2' }>(), { level: 'h2' })
</script>

<template>
  <div class="mx-auto max-w-2xl text-center">
    <p v-if="eyebrow" class="text-sm font-semibold uppercase tracking-wider text-brand-600">{{ eyebrow }}</p>
    <component :is="level" class="mt-2 text-3xl font-bold tracking-tight text-fg">{{ title }}</component>
    <p v-if="lead" class="mt-4 text-fg-muted">{{ lead }}</p>
  </div>
</template>
