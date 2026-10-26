<script setup lang="ts">
/**
 * Every markdown page except the home page - contact, privacy, terms, and
 * whatever a project adds. A new page is a pair of files in content/en and
 * content/pl, not a route.
 *
 * Every section is switched on by frontmatter, on any page: `plans:` makes it a
 * pricing page, `faqs:` gives it questions, and so on. The words live in the
 * content files; this template owns layout only. See
 * .ai/platform/docs/landing.md for each section's shape.
 */
const route = useRoute()
const { t } = useI18n()

// The content path, not the URL: `/pl/contact` is `contact.md` in content/pl,
// so the locale prefix must not reach the query. The route param never has it.
const path = computed(() => `/${[route.params.slug].flat().filter(Boolean).join('/')}`)
const { data: page } = await useContentPage(path)

if (!page.value) {
  throw createError({ statusCode: 404, statusMessage: t('site.notFound'), fatal: true })
}

usePageSeo(page)
</script>

<template>
  <div v-if="page">
    <article class="mx-auto max-w-3xl px-6 pt-16">
      <h1 class="text-4xl font-bold tracking-tight text-fg">{{ page.title }}</h1>
      <p class="mt-4 text-lg text-fg-muted">{{ page.description }}</p>
    </article>

    <div v-if="page.plans" class="mt-12">
      <PricingPlans :plans="page.plans" />
    </div>

    <article v-if="page.body.value.length" class="mx-auto max-w-3xl px-6 pb-16">
      <ContentRenderer :value="page" class="prose-site mt-10" />
    </article>

    <FeatureGrid v-if="page.features" :section="page.features" />
    <StepList v-if="page.steps" :section="page.steps" />
    <FaqList v-if="page.faqs" :section="page.faqs" />
    <CtaBand v-if="page.cta" :cta="page.cta" />
  </div>
</template>
