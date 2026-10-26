<script setup lang="ts">
const { t } = useI18n()
const { public: cfg } = useRuntimeConfig()
const year = new Date().getFullYear()
const localePath = useLocalePath()

const columns = computed(() => [
  {
    title: t('site.footer.product'),
    links: [
      { label: t('site.nav.contact'), to: '/contact' },
    ],
  },
  {
    title: t('site.footer.legal'),
    links: [
      { label: t('site.footer.privacy'), to: '/privacy' },
      { label: t('site.footer.terms'), to: '/terms' },
    ],
  },
])
</script>

<template>
  <footer class="border-t border-border bg-surface">
    <div class="mx-auto grid max-w-6xl gap-8 px-6 py-12 sm:grid-cols-2 md:grid-cols-4">
      <div class="md:col-span-2">
        <p class="text-lg font-semibold text-fg">{{ cfg.appName }}</p>
        <p class="mt-2 max-w-sm text-sm text-fg-muted">{{ t('site.footer.tagline') }}</p>
      </div>

      <div v-for="col in columns" :key="col.title">
        <h2 class="text-xs font-semibold uppercase tracking-wider text-fg">{{ col.title }}</h2>
        <ul class="mt-3 space-y-2">
          <li v-for="link in col.links" :key="link.to">
            <NuxtLink :to="localePath(link.to)" class="text-sm text-fg-muted hover:text-fg">{{ link.label }}</NuxtLink>
          </li>
        </ul>
      </div>
    </div>

    <div class="border-t border-border">
      <p class="mx-auto max-w-6xl px-6 py-4 text-xs text-fg-muted">
        {{ t('site.footer.rights', { year, name: cfg.appName }) }}
      </p>
    </div>
  </footer>
</template>
