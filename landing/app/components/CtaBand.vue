<script setup lang="ts">
import type { PagesEnCollectionItem } from '@nuxt/content'

defineProps<{ cta: NonNullable<PagesEnCollectionItem['cta']> }>()
</script>

<template>
  <section class="bg-brand-900">
    <div class="mx-auto max-w-4xl px-6 py-16 text-center">
      <h2 class="text-3xl font-bold tracking-tight text-white">{{ cta.title }}</h2>
      <p class="mx-auto mt-4 max-w-2xl text-brand-100">{{ cta.body }}</p>
      <SiteLink :to="cta.primary.to" class="mt-8 inline-block rounded-lg bg-white px-5 py-3 text-sm font-semibold text-brand-800 hover:bg-brand-50">
        {{ cta.primary.label }}
      </SiteLink>
    </div>
  </section>
</template>
