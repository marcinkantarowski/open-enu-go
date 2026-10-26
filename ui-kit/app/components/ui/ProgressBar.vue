<script setup lang="ts">
const props = defineProps<{ percent: number, label?: string, tone?: 'brand' | 'danger' | 'success' }>()

const tones: Record<string, string> = {
  brand: 'bg-brand-600',
  danger: 'bg-danger',
  success: 'bg-success',
}
</script>

<template>
  <div class="flex flex-col gap-1">
    <div
      class="h-2 w-full overflow-hidden rounded-full bg-surface-sunken"
      role="progressbar"
      :aria-valuenow="Math.round(props.percent)"
      aria-valuemin="0"
      aria-valuemax="100"
      :aria-label="props.label"
    >
      <div
        class="h-full rounded-full transition-[width] duration-300"
        :class="tones[props.tone ?? 'brand']"
        :style="{ width: `${Math.min(100, Math.max(0, props.percent))}%` }"
      />
    </div>
    <p v-if="props.label" class="text-xs text-fg-muted">{{ props.label }}</p>
  </div>
</template>
