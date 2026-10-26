<script setup lang="ts">
import type { PagesEnCollectionItem } from '@nuxt/content'

defineProps<{ section: NonNullable<PagesEnCollectionItem['faqs']> }>()
</script>

<template>
  <section class="mx-auto max-w-3xl px-6 py-20">
    <SectionHeading :eyebrow="section.eyebrow" :title="section.title" :lead="section.lead" />

    <div class="mt-10 divide-y divide-border rounded-card border border-border bg-surface">
      <details v-for="item in section.items" :key="item.q" class="group p-5">
        <summary class="flex cursor-pointer list-none items-center justify-between gap-4 font-medium text-fg">
          {{ item.q }}
          <span class="text-fg-muted transition group-open:rotate-45" aria-hidden="true">+</span>
        </summary>
        <p class="mt-3 text-sm text-fg-muted">{{ item.a }}</p>
      </details>
    </div>
  </section>
</template>
