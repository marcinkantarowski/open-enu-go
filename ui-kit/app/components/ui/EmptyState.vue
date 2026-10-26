<script setup lang="ts">
defineProps<{ title?: string, description?: string }>()
const { t } = useI18n()
</script>

<template>
  <div class="flex flex-col items-center gap-2 px-6 py-12 text-center">
    <p class="text-sm font-medium text-fg">{{ title ?? t('empty.title') }}</p>
    <p class="max-w-sm text-xs text-fg-muted">{{ description ?? t('empty.description') }}</p>
    <div v-if="$slots.default" class="mt-2">
      <slot />
    </div>
  </div>
</template>
