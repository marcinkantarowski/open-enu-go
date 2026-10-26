<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref, watch } from 'vue'

/**
 * Built on <dialog>, not a div with a high z-index.
 *
 * The native element gives focus trapping, Escape-to-close, inert background and
 * the top layer for free - all of which are the parts hand-rolled modals get
 * wrong, and all of which are accessibility requirements rather than polish.
 */
const open = defineModel<boolean>('open', { default: false })

defineProps<{ title?: string }>()

const dialog = ref<HTMLDialogElement | null>(null)

watch(open, (isOpen) => {
  if (!dialog.value) return
  if (isOpen && !dialog.value.open) dialog.value.showModal()
  if (!isOpen && dialog.value.open) dialog.value.close()
})

// Escape and the backdrop close the dialog without going through the model, so
// the model has to be told or it desynchronises and the next open does nothing.
const syncClose = () => { open.value = false }

onMounted(() => {
  dialog.value?.addEventListener('close', syncClose)
  if (open.value) dialog.value?.showModal()
})

onBeforeUnmount(() => dialog.value?.removeEventListener('close', syncClose))
</script>

<template>
  <dialog
    ref="dialog"
    class="m-auto w-[min(32rem,calc(100vw-2rem))] rounded-card border border-border bg-surface p-0 text-fg backdrop:bg-black/40"
    :aria-label="title"
  >
    <header v-if="title" class="border-b border-border px-5 py-4">
      <h2 class="text-sm font-semibold">{{ title }}</h2>
    </header>

    <div class="px-5 py-4">
      <slot />
    </div>

    <footer v-if="$slots.footer" class="flex justify-end gap-2 border-t border-border px-5 py-3">
      <slot name="footer" />
    </footer>
  </dialog>
</template>
