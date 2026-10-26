<script setup lang="ts">
import type { ConflictError } from '../composables/useApi'

/**
 * What a 409 looks like.
 *
 * Three explicit choices, no default. The usual "this record changed, please
 * reload" silently discards whatever the person just typed; the server sends
 * both versions and the saved record precisely so this bar can offer a real
 * decision instead.
 */
const props = defineProps<{ conflict: ConflictError | null, busy?: boolean }>()

const emit = defineEmits<{ reload: [], overwrite: [], dismiss: [] }>()

const { t } = useI18n()
</script>

<template>
  <div
    v-if="props.conflict"
    class="flex flex-col gap-3 rounded-card border border-warning bg-warning-soft px-4 py-3"
    role="alert"
    data-testid="conflict-bar"
  >
    <div>
      <p class="text-sm font-semibold text-fg">{{ t('conflict.title') }}</p>
      <p class="mt-0.5 text-xs text-fg-muted">
        {{ t('conflict.description', {
          yours: props.conflict.yourVersion ?? '?',
          current: props.conflict.currentVersion ?? '?',
        }) }}
      </p>
    </div>

    <!-- The saved record, verbatim. Without it "theirs" is an abstraction and
         nobody can tell which choice loses less. -->
    <dl v-if="props.conflict.current" class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-xs">
      <template v-for="(value, key) in props.conflict.current" :key="key">
        <dt class="font-medium text-fg-muted">{{ key }}</dt>
        <dd class="font-mono text-fg">{{ value }}</dd>
      </template>
    </dl>

    <div class="flex flex-wrap gap-2">
      <UiButton variant="primary" size="sm" :loading="props.busy" @click="emit('reload')">
        {{ t('conflict.reload') }}
      </UiButton>
      <UiButton variant="danger" size="sm" :loading="props.busy" @click="emit('overwrite')">
        {{ t('conflict.overwrite') }}
      </UiButton>
      <UiButton variant="ghost" size="sm" @click="emit('dismiss')">
        {{ t('conflict.dismiss') }}
      </UiButton>
    </div>
  </div>
</template>
