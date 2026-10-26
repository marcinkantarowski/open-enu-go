<script setup lang="ts">
import type { PagesEnCollectionItem } from '@nuxt/content'

defineProps<{ section: NonNullable<PagesEnCollectionItem['steps']> }>()
</script>

<template>
  <section class="bg-surface-muted">
    <div class="mx-auto max-w-6xl px-6 py-20">
      <SectionHeading :eyebrow="section.eyebrow" :title="section.title" :lead="section.lead" />

      <ol class="mt-12 grid gap-8 md:grid-cols-3">
        <li v-for="(step, i) in section.items" :key="step.title" class="text-center">
          <span class="mx-auto flex size-10 items-center justify-center rounded-full bg-brand-600 font-semibold text-white">
            {{ i + 1 }}
          </span>
          <h3 class="mt-4 font-semibold text-fg">{{ step.title }}</h3>
          <p class="mt-2 text-sm text-fg-muted">{{ step.body }}</p>
        </li>
      </ol>
    </div>
  </section>
</template>
