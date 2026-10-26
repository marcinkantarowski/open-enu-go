<script setup lang="ts">
const { settingsTabs } = useNavigation()
const { t } = useI18n()
</script>

<template>
  <div class="flex flex-col gap-4">
    <h1 class="text-lg font-semibold text-fg">{{ t('settings.title') }}</h1>

    <nav class="flex flex-wrap gap-1 border-b border-border" data-testid="settings-tabs">
      <NuxtLink
        v-for="tab in settingsTabs"
        :key="tab.to"
        :to="tab.to"
        class="-mb-px border-b-2 border-transparent px-3 py-2 text-sm text-fg-muted hover:text-fg"
        active-class="border-brand-600 text-brand-700"
      >
        {{ tab.label }}
      </NuxtLink>
    </nav>
  </div>
</template>
