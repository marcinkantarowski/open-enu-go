<script setup lang="ts">
const { t, locale, locales, setLocale } = useI18n()
const { public: cfg } = useRuntimeConfig()
const route = useRoute()
const localePath = useLocalePath()

const open = ref(false)
watch(() => route.fullPath, () => { open.value = false })

// A project adds its pages here as it grows them - see .ai/platform/docs/landing.md.
const nav = computed(() => [
  { label: t('site.nav.contact'), to: '/contact' },
])
</script>

<template>
  <header class="sticky top-0 z-50 border-b border-border bg-surface/90 backdrop-blur">
    <div class="mx-auto flex max-w-6xl items-center justify-between gap-6 px-6 py-3">
      <NuxtLink :to="localePath('/')" class="text-lg font-semibold tracking-tight text-fg" :aria-label="t('site.nav.home')">
        {{ cfg.appName }}
      </NuxtLink>

      <button
        type="button"
        class="rounded-md px-2 py-1 text-fg-muted hover:bg-surface-sunken md:hidden"
        :aria-label="t('site.nav.menu')"
        :aria-expanded="open"
        @click="open = !open"
      >
        ☰
      </button>

      <nav
        class="absolute inset-x-0 top-full flex-col gap-1 border-b border-border bg-surface px-6 pb-4 md:static md:flex md:flex-row md:items-center md:gap-6 md:border-0 md:bg-transparent md:p-0"
        :class="open ? 'flex' : 'hidden'"
      >
        <NuxtLink
          v-for="item in nav"
          :key="item.to"
          :to="localePath(item.to)"
          class="py-2 text-sm font-medium text-fg-muted hover:text-fg md:py-0"
          active-class="text-brand-700"
        >
          {{ item.label }}
        </NuxtLink>

        <select
          :value="locale"
          class="rounded-md border border-border bg-surface py-1 pl-2 pr-6 text-sm text-fg-muted"
          :aria-label="t('site.nav.language')"
          @change="setLocale(($event.target as HTMLSelectElement).value as typeof locale)"
        >
          <!-- `selected` as well as the select's `:value`: the server renders options, not
               the select's value, so without it the first language shows until hydration. -->
          <option v-for="l in locales" :key="l.code" :value="l.code" :selected="l.code === locale">
            {{ t(`site.locale.${l.code}`) }}
          </option>
        </select>

        <a :href="`${cfg.appUrl}/login`" class="py-2 text-sm font-medium text-fg hover:text-brand-700 md:py-0" rel="nofollow">
          {{ t('site.nav.signIn') }}
        </a>
        <a
          :href="`${cfg.appUrl}/register`"
          class="rounded-lg bg-brand-600 px-3.5 py-2 text-center text-sm font-medium text-white hover:bg-brand-700"
          rel="nofollow"
        >
          {{ t('site.nav.getStarted') }}
        </a>
      </nav>
    </div>
  </header>
</template>
