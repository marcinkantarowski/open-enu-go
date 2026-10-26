<script setup lang="ts">
/**
 * A link written in content: `/contact` stays in the site - in the visitor's
 * language, so `/pl/contact` for a Polish reader - and `app:/register` leaves
 * for the app host. See useSiteLink().
 */
const props = defineProps<{ to: string }>()
const resolve = useSiteLink()
const localePath = useLocalePath()
const target = computed(() => resolve(props.to))
</script>

<template>
  <a v-if="target.external" :href="target.href" rel="nofollow"><slot /></a>
  <NuxtLink v-else :to="localePath(target.href)"><slot /></NuxtLink>
</template>
