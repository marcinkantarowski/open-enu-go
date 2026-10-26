<script setup lang="ts">
import { useId } from 'vue'

/**
 * Label, control, hint and error, wired together.
 *
 * The wiring is the point: `for`/`id` and `aria-describedby` are what make an
 * error message reach someone using a screen reader, and they are exactly what
 * gets forgotten when every form does it by hand.
 */
const props = defineProps<{
  label: string
  hint?: string
  error?: string | null
  required?: boolean
}>()

const id = useId()
const describedBy = computed(() => {
  const ids = []
  if (props.error) ids.push(`${id}-error`)
  else if (props.hint) ids.push(`${id}-hint`)
  return ids.join(' ') || undefined
})
</script>

<template>
  <div class="flex flex-col gap-1.5">
    <label :for="id" class="text-sm font-medium text-fg">
      {{ props.label }}
      <span v-if="props.required" class="text-danger" aria-hidden="true">*</span>
    </label>

    <slot :id="id" :described-by="describedBy" :invalid="Boolean(props.error)" />

    <p v-if="props.error" :id="`${id}-error`" class="text-xs text-danger">{{ props.error }}</p>
    <p v-else-if="props.hint" :id="`${id}-hint`" class="text-xs text-fg-muted">{{ props.hint }}</p>
  </div>
</template>
