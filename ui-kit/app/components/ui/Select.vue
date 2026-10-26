<script setup lang="ts">
const model = defineModel<string>({ default: '' })

defineProps<{
  id?: string
  options: Array<{ value: string, label: string }>
  disabled?: boolean
  describedBy?: string
}>()
</script>

<template>
  <select
    :id="id"
    v-model="model"
    :disabled="disabled"
    :aria-describedby="describedBy"
    class="w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm text-fg shadow-card transition-colors hover:border-brand-300 focus:border-brand-500 focus:outline-none focus:ring-3 focus:ring-brand-100 disabled:opacity-60"
  >
    <option v-for="option in options" :key="option.value" :value="option.value">
      {{ option.label }}
    </option>
  </select>
</template>
