<script setup lang="ts">
/**
 * Links written in a markdown BODY. Without this override they render as plain
 * anchors to `/privacy`, which takes a Polish reader to the English page - and
 * tells a crawler the Polish page links to the English one.
 */
defineProps<{ href?: string, target?: string }>()
</script>

<template>
  <a v-if="!href || /^([a-z]+:|#|\/\/)/i.test(href) && !href.startsWith('app:')" :href="href" :target="target" rel="noopener"><slot /></a>
  <SiteLink v-else :to="href"><slot /></SiteLink>
</template>
