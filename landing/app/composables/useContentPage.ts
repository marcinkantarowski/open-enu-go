const collections = { en: 'pages_en', pl: 'pages_pl' } as const

type Locale = keyof typeof collections

/**
 * The page at `path`, in the visitor's language.
 *
 * Deliberately no fallback to English: a missing translation would render in
 * the wrong language to exactly the people who chose the other one, and nobody
 * reading the English site would notice. `make i18n-check` fails instead.
 *
 * The key includes the locale, so switching language refetches.
 */
export function useContentPage(path: MaybeRefOrGetter<string>) {
  const { locale } = useI18n()

  return useAsyncData(
    () => `page:${locale.value}:${toValue(path)}`,
    () => queryCollection(collections[locale.value as Locale] ?? collections.en)
      .path(toValue(path))
      .first(),
  )
}

/**
 * Content links are written as `/contact` or `app:/register`.
 *
 * Markdown cannot know the app's host - it derives from `DOMAIN` - so the
 * prefix is resolved here, at render time, from runtime config.
 */
export function useSiteLink() {
  const { public: cfg } = useRuntimeConfig()

  return (to: string) => to.startsWith('app:')
    ? { href: `${cfg.appUrl}${to.slice(4)}`, external: true }
    : { href: to, external: false }
}
