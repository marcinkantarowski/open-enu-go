<script setup lang="ts">
defineProps<{ title?: string, description?: string }>()
</script>

<template>
  <section class="rounded-card border border-border bg-surface shadow-card">
    <header v-if="title || $slots.header" class="flex items-start justify-between gap-4 border-b border-border px-6 py-4">
      <div>
        <h2 v-if="title" class="text-sm font-semibold text-fg">{{ title }}</h2>
        <p v-if="description" class="mt-0.5 text-xs text-fg-muted">{{ description }}</p>
      </div>
      <slot name="header" />
    </header>

    <div class="px-6 py-5">
      <slot />
    </div>

    <footer v-if="$slots.footer" class="border-t border-border px-6 py-3">
      <slot name="footer" />
    </footer>
  </section>
</template>
