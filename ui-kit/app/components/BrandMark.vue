<script setup lang="ts">
/**
 * The product's mark: its initial on the brand colour, and optionally its name.
 *
 * A letter rather than an image so that a project renamed by `make init` has a
 * mark that is already its own. Replace this one component to use a real logo;
 * every shell and sign-in screen renders it.
 */
const props = withDefaults(defineProps<{ name: string, tone?: 'brand' | 'inverse', showName?: boolean }>(), {
  tone: 'brand',
  showName: true,
})

const initial = computed(() => props.name.trim().charAt(0).toUpperCase())
</script>

<template>
  <span class="inline-flex items-center gap-2.5">
    <span
      class="inline-flex size-8 shrink-0 items-center justify-center rounded-lg text-sm font-bold"
      :class="props.tone === 'inverse' ? 'bg-white/15 text-white ring-1 ring-white/25' : 'bg-brand-600 text-white'"
      aria-hidden="true"
    >{{ initial }}</span>
    <span
      v-if="props.showName"
      class="text-sm font-semibold tracking-tight"
      :class="props.tone === 'inverse' ? 'text-white' : 'text-fg'"
    >{{ props.name }}</span>
  </span>
</template>
