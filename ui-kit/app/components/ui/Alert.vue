<script setup lang="ts">
const props = withDefaults(defineProps<{ tone?: 'info' | 'success' | 'warning' | 'danger', title?: string }>(), {
  tone: 'info',
})

const tones: Record<string, string> = {
  info: 'border-brand-200 bg-brand-50 text-brand-900',
  success: 'border-success bg-success-soft text-fg',
  warning: 'border-warning bg-warning-soft text-fg',
  danger: 'border-danger bg-danger-soft text-fg',
}
</script>

<template>
  <!-- `alert` for the two tones someone must not miss; `status` for the rest,
       which announce politely instead of interrupting. -->
  <div
    class="rounded-lg border px-4 py-3 text-sm"
    :class="tones[props.tone]"
    :role="props.tone === 'danger' || props.tone === 'warning' ? 'alert' : 'status'"
  >
    <p v-if="props.title" class="font-semibold">{{ props.title }}</p>
    <slot />
  </div>
</template>
