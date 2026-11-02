import type { PagesEnCollectionItem } from '@nuxt/content'

type Page = PagesEnCollectionItem

/**
 * The company behind the product, for crawlers: `organization` in the landing's
 * app.config.ts. Without one the publisher is the product alone.
 */
interface Organization {
  legalName: string
  url?: string
  email?: string
  foundingDate?: string
  taxId?: string
  /** Registry numbers by register, `{ KRS: '0000723560' }`. */
  identifiers?: Record<string, string>
  address?: { street: string, postalCode: string, city: string, country: string }
}

interface Options {
  /** The page this one sits under - an article's list - between home and it in the breadcrumb. */
  parent?: MaybeRefOrGetter<{ name: string, path: string } | undefined>
  /** The share image when the page names none in `image`; /og.png when this is empty too. */
  image?: MaybeRefOrGetter<string | undefined>
}

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
export function usePageSeo(page: Ref<Page | null | undefined>, options: Options = {}) {
  const { public: cfg } = useRuntimeConfig()
  const { locale, locales } = useI18n()
  const route = useRoute()
  const switchLocalePath = useSwitchLocalePath()
  const localePath = useLocalePath()
  // Read here, not in jsonLd(): in the browser useHead resolves the JSON-LD at once.
  const org = (useAppConfig() as { organization?: Organization }).organization

  // No query string, no trailing slash: one page, one URL.
  const absolute = (path: string) => `${cfg.siteUrl}${path.replace(/\/+$/, '') || '/'}`
  const isHome = computed(() => page.value?.path === '/')
  const canonical = computed(() => absolute(route.path))
  const language = (code: string) => locales.value.find(l => l.code === code)?.language ?? code

  // `headTitle` when the page sets one - a heading too long for a search result keeps
  // its words on the page and gives the result a shorter line. Otherwise the home page
  // leads with its hero (its own title is the product's name, which the template
  // already appends), and every other page with its title.
  const title = computed(() => page.value?.headTitle ?? (isHome.value ? page.value?.hero?.title : undefined) ?? page.value?.title)
  const image = computed(() => absolute(page.value?.image ?? toValue(options.image) ?? '/og.png'))
  const indexable = computed(() => cfg.indexable && !page.value?.noindex)

  useSeoMeta({
    title,
    description: () => page.value?.description,
    robots: () => (indexable.value ? 'index, follow' : 'noindex, nofollow'),
    ogTitle: title,
    ogDescription: () => page.value?.description,
    ogType: () => (page.value?.published ? 'article' : 'website'),
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
      { rel: 'alternate' as const, hreflang: 'x-default', href: absolute(switchLocalePath(cfg.defaultLocale as typeof locale.value)) },
    ],
    script: () => [{ type: 'application/ld+json', innerHTML: JSON.stringify(jsonLd()) }],
  })

  function jsonLd() {
    // The publisher: the product, under the company that makes it when app.config.ts
    // names one, so a crawler can tie a page to a real business with an address.
    const organization = {
      '@type': 'Organization',
      '@id': `${cfg.siteUrl}/#organization`,
      name: cfg.appName,
      url: cfg.siteUrl,
      // A square raster: what a search engine accepts as a logo (ui-kit's public/).
      logo: absolute('/apple-touch-icon.png'),
      ...(org ? { email: org.email, parentOrganization: company(org) } : {}),
    }
    const publisher = { '@id': organization['@id'] }
    const website = { '@type': 'WebSite', '@id': `${cfg.siteUrl}/#website`, name: cfg.appName, url: cfg.siteUrl, inLanguage: language(locale.value), publisher }

    const graph: Record<string, unknown>[] = isHome.value
      ? [
          { ...website, description: page.value?.description },
          organization,
          // The product itself: a web application, bought as a service.
          { '@type': 'SoftwareApplication', name: cfg.appName, url: cfg.appUrl, applicationCategory: 'BusinessApplication', operatingSystem: 'Web', description: page.value?.description, inLanguage: language(locale.value), publisher },
        ]
      : page.value?.published
        ? [{ '@type': 'Article', headline: page.value.title, url: canonical.value, mainEntityOfPage: canonical.value, description: page.value.description, inLanguage: language(locale.value), datePublished: page.value.published, dateModified: page.value.updated ?? page.value.published, image: image.value, keywords: page.value.tags?.join(', '), author: publisher, publisher, isPartOf: website }, organization]
        : [{ '@type': 'WebPage', name: page.value?.title, url: canonical.value, description: page.value?.description, inLanguage: language(locale.value), isPartOf: website, publisher }, organization]

    // Where the page sits: home, the page it is under when there is one, then the page.
    if (!isHome.value && page.value) {
      const parent = toValue(options.parent)
      const trail = [
        { name: cfg.appName as string, url: absolute(localePath('/')) },
        ...(parent ? [{ name: parent.name, url: absolute(localePath(parent.path)) }] : []),
        { name: page.value.title, url: canonical.value },
      ]
      graph.push({ '@type': 'BreadcrumbList', itemListElement: trail.map((c, i) => ({ '@type': 'ListItem', position: i + 1, name: c.name, item: c.url })) })
    }

    if (page.value?.faqs) {
      graph.push({
        '@type': 'FAQPage',
        mainEntity: page.value.faqs.items.map(f => ({ '@type': 'Question', name: f.q, acceptedAnswer: { '@type': 'Answer', text: f.a } })),
      })
    }
    return { '@context': 'https://schema.org', '@graph': graph }
  }
}

function company(o: Organization) {
  return {
    '@type': 'Organization',
    name: o.legalName,
    legalName: o.legalName,
    url: o.url,
    foundingDate: o.foundingDate,
    taxID: o.taxId,
    identifier: o.identifiers && Object.entries(o.identifiers).map(([propertyID, value]) => ({ '@type': 'PropertyValue', propertyID, value })),
    address: o.address && { '@type': 'PostalAddress', streetAddress: o.address.street, postalCode: o.address.postalCode, addressLocality: o.address.city, addressCountry: o.address.country },
  }
}
