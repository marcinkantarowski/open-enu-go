<script setup lang="ts">
/**
 * The one button.
 *
 * Variants rather than free-form classes, so "what does a destructive action
 * look like?" has a single answer that can be changed once.
 */
const props = withDefaults(defineProps<{
  variant?: 'primary' | 'secondary' | 'ghost' | 'danger'
  size?: 'sm' | 'md' | 'lg'
  type?: 'button' | 'submit'
  disabled?: boolean
  loading?: boolean
}>(), {
  variant: 'secondary',
  size: 'md',
  type: 'button',
  disabled: false,
  loading: false,
})

const variants: Record<string, string> = {
  primary: 'bg-brand-600 text-white shadow-card hover:bg-brand-700 active:bg-brand-800 border-transparent',
  secondary: 'bg-surface text-fg shadow-card hover:bg-surface-sunken border-border',
  ghost: 'bg-transparent text-fg-muted hover:text-fg hover:bg-surface-sunken border-transparent',
  danger: 'bg-danger text-white hover:brightness-95 border-transparent',
}

const sizes: Record<string, string> = {
  sm: 'px-2.5 py-1 text-xs',
  md: 'px-3.5 py-2 text-sm',
  // The one action a whole screen exists for - a sign-in, a submit.
  lg: 'px-4 py-2.5 text-sm',
}
</script>

<template>
  <button
    :type="props.type"
    :disabled="props.disabled || props.loading"
    :aria-busy="props.loading"
    class="inline-flex items-center justify-center gap-2 rounded-lg border font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50"
    :class="[variants[props.variant], sizes[props.size]]"
  >
    <UiSpinner v-if="props.loading" class="size-4" />
    <slot />
  </button>
</template>
