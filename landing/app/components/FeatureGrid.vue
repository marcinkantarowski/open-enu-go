<script setup lang="ts">
import type { PagesEnCollectionItem } from '@nuxt/content'

defineProps<{ section: NonNullable<PagesEnCollectionItem['features']> }>()
</script>

<template>
  <section class="mx-auto max-w-6xl px-6 py-20">
    <SectionHeading :eyebrow="section.eyebrow" :title="section.title" :lead="section.lead" />

    <div class="mt-12 grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
      <article
        v-for="item in section.items"
        :key="item.title"
        class="rounded-card border border-border bg-surface p-6 shadow-sm"
      >
        <span class="text-2xl" aria-hidden="true">{{ item.icon }}</span>
        <h3 class="mt-3 font-semibold text-fg">{{ item.title }}</h3>
        <p class="mt-2 text-sm text-fg-muted">{{ item.body }}</p>
      </article>
    </div>
  </section>
</template>
