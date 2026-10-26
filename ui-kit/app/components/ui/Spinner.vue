<script setup lang="ts">
// Decorative: the surrounding control carries aria-busy, so a screen reader
// hears "busy" once rather than announcing a nameless graphic.
</script>

<template>
  <svg class="size-4 animate-spin" viewBox="0 0 24 24" fill="none" aria-hidden="true">
    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="3" />
    <path class="opacity-90" fill="currentColor" d="M12 2a10 10 0 0 1 10 10h-3a7 7 0 0 0-7-7V2Z" />
  </svg>
</template>
