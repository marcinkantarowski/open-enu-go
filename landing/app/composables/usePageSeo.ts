import type { PagesEnCollectionItem } from '@nuxt/content'

type Page = PagesEnCollectionItem

/**
 * Everything a crawler reads from the head of a page, in one place: title,
 * description, canonical, hreflang alternates, Open Graph, Twitter card, robots
 * and JSON-LD. Both page templates call it; a template that does not has no
 * SEO at all.
 *
 * URLs are absolute and built from `siteUrl`, never from the request: the site
 * also answers on `www.`, and the canonical URL is what tells a search engine
 * those are one page and not two.
 */
export function usePageSeo(page: Ref<Page | null | undefined>) {
  const { public: cfg } = useRuntimeConfig()
  const { locale, locales } = useI18n()
  const route = useRoute()
  const switchLocalePath = useSwitchLocalePath()

  // No query string, no trailing slash: one page, one URL.
  const absolute = (path: string) => `${cfg.siteUrl}${path.replace(/\/+$/, '') || '/'}`
  const isHome = computed(() => page.value?.path === '/')
  const canonical = computed(() => absolute(route.path))
  const language = (code: string) => locales.value.find(l => l.code === code)?.language ?? code

  // The home page's own title is the product's name, which the template already
  // appends - so its <title> leads with what the page says instead.
  const title = computed(() => (isHome.value ? page.value?.hero?.title : undefined) ?? page.value?.title)
  const image = computed(() => absolute(page.value?.image ?? '/og.png'))
  const indexable = computed(() => cfg.indexable && !page.value?.noindex)

  useSeoMeta({
    title,
    description: () => page.value?.description,
    robots: () => (indexable.value ? 'index, follow' : 'noindex, nofollow'),
    ogTitle: title,
    ogDescription: () => page.value?.description,
    ogType: 'website',
    ogUrl: canonical,
    ogSiteName: cfg.appName as string,
    ogImage: image,
    ogLocale: () => language(locale.value).replace('-', '_'),
    ogLocaleAlternate: () => locales.value.filter(l => l.code !== locale.value).map(l => language(l.code).replace('-', '_')),
    twitterCard: 'summary_large_image',
  })

  useHead({
    link: () => [
      { rel: 'canonical' as const, href: canonical.value },
      ...locales.value.map(l => ({ rel: 'alternate' as const, hreflang: language(l.code), href: absolute(switchLocalePath(l.code)) })),
      { rel: 'alternate' as const, hreflang: 'x-default', href: absolute(switchLocalePath(cfg.defaultLocale as 'en' | 'pl')) },
    ],
    script: () => [{ type: 'application/ld+json', innerHTML: JSON.stringify(jsonLd()) }],
  })

  function jsonLd() {
    const organization = { '@type': 'Organization', name: cfg.appName, url: cfg.siteUrl }
    const graph: Record<string, unknown>[] = isHome.value
      ? [{ '@type': 'WebSite', name: cfg.appName, url: cfg.siteUrl, description: page.value?.description, inLanguage: language(locale.value), publisher: organization }]
      : [{ '@type': 'WebPage', name: page.value?.title, url: canonical.value, description: page.value?.description, inLanguage: language(locale.value), isPartOf: { '@type': 'WebSite', name: cfg.appName, url: cfg.siteUrl } }]

    if (page.value?.faqs) {
      graph.push({
        '@type': 'FAQPage',
        mainEntity: page.value.faqs.items.map(f => ({ '@type': 'Question', name: f.q, acceptedAnswer: { '@type': 'Answer', text: f.a } })),
      })
    }
    return { '@context': 'https://schema.org', '@graph': graph }
  }
}
