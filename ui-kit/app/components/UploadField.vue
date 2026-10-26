<script setup lang="ts">
import { ref } from 'vue'
import type { UploadedFile } from '../composables/useUpload'

/**
 * Pick a file, see it upload, get back an attachment id.
 *
 * The size check is client-side ONLY as a courtesy - the server enforces it
 * regardless. Its value is that someone on a slow connection finds out before
 * spending four minutes uploading something that will be refused.
 */
const props = withDefaults(defineProps<{
  owner?: { type: string, id: string }
  accept?: string
  /** Mirror of the server's limit, purely so the rejection arrives sooner. */
  maxBytes?: number
}>(), {
  accept: undefined,
  maxBytes: 10 * 1024 * 1024,
})

const emit = defineEmits<{ uploaded: [file: UploadedFile] }>()

const { upload, progress, uploading, error } = useUpload()
const { t } = useI18n()
const input = ref<HTMLInputElement | null>(null)
const localError = ref<string | null>(null)

async function onPick(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0]
  if (!file) return

  localError.value = null

  if (file.size > props.maxBytes) {
    localError.value = t('upload.tooLarge', { max: `${Math.round(props.maxBytes / 1024 / 1024)} MB` })
    // Cleared so picking the same file again still fires a change event.
    if (input.value) input.value.value = ''
    return
  }

  try {
    emit('uploaded', await upload(file, props.owner))
  } finally {
    if (input.value) input.value.value = ''
  }
}
</script>

<template>
  <div class="flex flex-col gap-2">
    <label
      class="flex cursor-pointer flex-col items-center gap-1 rounded-card border border-dashed border-border bg-surface px-4 py-6 text-center hover:bg-surface-muted"
    >
      <span class="text-sm font-medium text-fg">{{ t('upload.choose') }}</span>
      <span class="text-xs text-fg-muted">{{ t('upload.drop') }}</span>
      <input
        ref="input"
        type="file"
        class="sr-only"
        :accept="props.accept"
        :disabled="uploading"
        @change="onPick"
      >
    </label>

    <UiProgressBar
      v-if="uploading"
      :percent="progress"
      :label="t('upload.uploading', { percent: progress })"
    />

    <p v-if="localError || error" class="text-xs text-danger" role="alert">{{ localError ?? error }}</p>
  </div>
</template>
