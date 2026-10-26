<script setup lang="ts">
/**
 * A download link for one attachment.
 *
 * The URL is fetched on click rather than rendered up front, because it is
 * signed and short-lived: a URL that works forever is a credential, and a page
 * that renders fifty of them has leaked fifty.
 */
const props = defineProps<{ id: string, filename: string }>()

const api = useApi()
const toast = useToast()
const busy = ref(false)

async function open() {
  busy.value = true

  try {
    const { url } = await api.get<{ url: string }>(`/api/attachments/${props.id}`)
    window.open(url, '_blank', 'noopener')
  } catch (error) {
    toast.error((error as Error).message)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <UiButton variant="ghost" size="sm" :loading="busy" @click="open">
    {{ props.filename }}
  </UiButton>
</template>
