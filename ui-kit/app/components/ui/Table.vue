<script setup lang="ts" generic="Row extends object">
/**
 * A table with the parts that are always needed and always rebuilt: column
 * definitions, a loading state that does not collapse the layout, and an empty
 * state that is not a blank rectangle.
 *
 * Generic over the row type so a page passes its own interface and the
 * `#cell-*` slots receive it typed, rather than every call site casting through
 * `Record<string, unknown>`.
 */
export interface Column {
  key: string
  label: string
  /** Right-aligned for anything numeric, so digits line up to be compared. */
  numeric?: boolean
}

defineProps<{
  columns: Column[]
  rows: readonly Row[]
  rowKey?: string
  loading?: boolean
  emptyTitle?: string
}>()

defineSlots<{
  [key: `cell-${string}`]: (props: { row: Row, value: unknown }) => unknown
}>()

/** Columns are addressed by name, which the row type cannot express. */
const cell = (row: Row, key: string): unknown => (row as Record<string, unknown>)[key]
</script>

<template>
  <div class="overflow-x-auto">
    <table class="w-full border-collapse text-sm">
      <thead>
        <tr class="border-b border-border bg-surface-muted text-left text-xs uppercase tracking-wide text-fg-muted">
          <th
            v-for="column in columns"
            :key="column.key"
            scope="col"
            class="px-4 py-2.5 font-semibold"
            :class="column.numeric ? 'text-right' : ''"
          >
            {{ column.label }}
          </th>
        </tr>
      </thead>

      <tbody>
        <tr v-if="loading">
          <td :colspan="columns.length" class="px-4 py-8 text-center text-fg-muted">
            <UiSpinner class="mx-auto size-5" />
          </td>
        </tr>

        <tr v-else-if="rows.length === 0">
          <td :colspan="columns.length">
            <UiEmptyState :title="emptyTitle" />
          </td>
        </tr>

        <!-- v-for lives inside the v-else template: Vue gives v-if higher
             precedence than v-for on the same element, so the two together read
             as working by accident rather than by intent. -->
        <template v-else>
          <tr
            v-for="(row, index) in rows"
            :key="String(cell(row, rowKey ?? 'id') ?? index)"
            class="border-b border-border last:border-0 hover:bg-surface-muted"
          >
            <td
              v-for="column in columns"
              :key="column.key"
              class="px-4 py-3 align-top text-fg"
              :class="column.numeric ? 'text-right tabular-nums' : ''"
            >
              <slot :name="`cell-${column.key}`" :row="row" :value="cell(row, column.key)">
                {{ cell(row, column.key) ?? '-' }}
              </slot>
            </td>
          </tr>
        </template>
      </tbody>
    </table>
  </div>
</template>
