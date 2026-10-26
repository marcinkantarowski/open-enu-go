<script setup lang="ts">
const props = withDefaults(defineProps<{ tone?: 'neutral' | 'success' | 'warning' | 'danger' | 'brand' }>(), {
  tone: 'neutral',
})

const tones: Record<string, string> = {
  neutral: 'bg-surface-sunken text-fg-muted',
  success: 'bg-success-soft text-success',
  warning: 'bg-warning-soft text-warning',
  danger: 'bg-danger-soft text-danger',
  brand: 'bg-brand-50 text-brand-700',
}
</script>

<template>
  <span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium" :class="tones[props.tone]">
    <slot />
  </span>
</template>
