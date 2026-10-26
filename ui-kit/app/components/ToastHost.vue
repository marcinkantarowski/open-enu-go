<script setup lang="ts">
/**
 * Mounted once, in the layout. Anything can raise a toast from anywhere because
 * the queue is module-scoped, not passed down through props.
 */
const { toasts, dismiss } = useToast()
const { t } = useI18n()

const tones: Record<string, string> = {
  info: 'border-border bg-surface',
  success: 'border-success bg-success-soft',
  error: 'border-danger bg-danger-soft',
}
</script>

<template>
  <!-- `polite`, not `assertive`: a success toast should not interrupt whatever
       a screen-reader user is in the middle of reading. -->
  <div
    class="pointer-events-none fixed bottom-4 right-4 z-50 flex w-[min(24rem,calc(100vw-2rem))] flex-col gap-2"
    role="status"
    aria-live="polite"
  >
    <div
      v-for="toast in toasts"
      :key="toast.id"
      class="pointer-events-auto flex items-start justify-between gap-3 rounded-card border px-4 py-3 text-sm shadow-lg"
      :class="tones[toast.tone]"
      data-testid="toast"
    >
      <span class="text-fg">{{ toast.message }}</span>
      <button
        type="button"
        class="text-xs text-fg-muted hover:text-fg"
        :aria-label="t('common.close')"
        @click="dismiss(toast.id)"
      >
        ✕
      </button>
    </div>
  </div>
</template>
