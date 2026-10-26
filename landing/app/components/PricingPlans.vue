<script setup lang="ts">
import type { PagesEnCollectionItem } from '@nuxt/content'

defineProps<{ plans: NonNullable<PagesEnCollectionItem['plans']> }>()
const { t } = useI18n()
</script>

<template>
  <div class="mx-auto grid max-w-6xl gap-6 px-6 md:grid-cols-3">
    <article
      v-for="plan in plans"
      :key="plan.name"
      class="flex flex-col rounded-card border bg-surface p-8 shadow-sm"
      :class="plan.highlighted ? 'border-brand-500 ring-2 ring-brand-500' : 'border-border'"
    >
      <div class="flex items-center justify-between gap-2">
        <h2 class="text-lg font-semibold text-fg">{{ plan.name }}</h2>
        <span v-if="plan.highlighted" class="rounded-full bg-brand-50 px-2.5 py-0.5 text-xs font-semibold text-brand-700">
          {{ t('site.pricing.popular') }}
        </span>
      </div>
      <p class="mt-2 text-sm text-fg-muted">{{ plan.description }}</p>

      <p class="mt-6 text-4xl font-bold tracking-tight text-fg">{{ plan.price }}</p>
      <p class="mt-1 min-h-5 text-sm text-fg-muted">{{ plan.period }}</p>

      <ul class="mt-6 flex-1 space-y-2">
        <li v-for="feature in plan.features" :key="feature" class="flex gap-2 text-sm text-fg-muted">
          <span class="text-success" aria-hidden="true">✓</span>{{ feature }}
        </li>
      </ul>

      <SiteLink
        :to="plan.cta.to"
        class="mt-8 rounded-lg px-4 py-2.5 text-center text-sm font-semibold"
        :class="plan.highlighted ? 'bg-brand-600 text-white hover:bg-brand-700' : 'border border-border text-fg hover:bg-surface-sunken'"
      >
        {{ plan.cta.label }}
      </SiteLink>
    </article>
  </div>
</template>
