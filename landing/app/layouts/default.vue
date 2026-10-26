<template>
  <div class="flex min-h-screen flex-col bg-surface">
    <SiteHeader />
    <main class="flex-1">
      <slot />
    </main>
    <SiteFooter />
  </div>
</template>
