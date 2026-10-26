<script setup lang="ts">
import { injectionsFor } from '../composables/defineInjection'

/**
 * A named hole in a page that other modules may fill.
 *
 *     <InjectionPoint name="example.project.sidebar" :context="{ item }" />
 *
 * This is the frontend half of the extension-surface rule
 * (.ai/platform/docs/frontend-conventions.md): a module adds a widget to
 * someone else's page by registering for a named spot, never by editing that
 * page. Two modules can then extend the same screen without touching the same
 * file, which is what keeps a merge from being a negotiation.
 *
 * Unknown names render nothing. That is deliberate - a spot with no
 * contributors is the normal case, not a misconfiguration.
 */
const props = defineProps<{ name: string, context?: Record<string, unknown> }>()

const widgets = computed(() => injectionsFor(props.name))
</script>

<template>
  <component
    :is="widget.component"
    v-for="widget in widgets"
    :key="widget.id"
    v-bind="props.context"
  />
</template>
