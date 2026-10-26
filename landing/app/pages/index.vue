<script setup lang="ts">
const { data: page } = await useContentPage('/')

usePageSeo(page)
</script>

<template>
  <div v-if="page">
    <HeroSection v-if="page.hero" :hero="page.hero" />
    <FeatureGrid v-if="page.features" :section="page.features" />
    <StepList v-if="page.steps" :section="page.steps" />

    <section v-if="page.body.value.length" class="mx-auto max-w-3xl px-6 py-20">
      <ContentRenderer :value="page" class="prose-site" />
    </section>

    <FaqList v-if="page.faqs" :section="page.faqs" />
    <CtaBand v-if="page.cta" :cta="page.cta" />
  </div>
</template>
