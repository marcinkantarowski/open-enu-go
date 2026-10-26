// Explicit, not auto-imported: the app-side helper of the same name takes no
// event, and which of the two a bare name resolves to depends on the tsconfig.
import { queryCollection } from '@nuxt/content/server'

// The request event, typed from the function that needs it rather than from
// `h3` - which is a dependency of Nuxt's, not one this app declares.
type H3Event = Parameters<typeof queryCollection>[0]

/**
 * The site as a crawler sees it: every page, in every language, with the URL
 * it is served at. robots.txt, sitemap.xml and llms.txt are all views of this
 * one list, so a page added to content/ appears in all three with no further
 * step - and one marked `noindex` disappears from all three.
 */

// The default locale has no URL prefix (`prefix_except_default` in nuxt.config),
// and which locale that is comes from DEFAULT_LOCALE - so the prefix is worked
// out per request instead of written down next to each language.
const LOCALES = [
  { code: 'en', language: 'en-GB', collection: 'pages_en' },
  { code: 'pl', language: 'pl-PL', collection: 'pages_pl' },
] as const

export interface SitePage {
  /** Content path, the same in every language: `/`, `/contact`. */
  path: string
  updated?: string
  versions: { code: string, language: string, url: string, title: string, description: string }[]
}

export function siteConfig(event: H3Event) {
  const { public: cfg } = useRuntimeConfig(event)
  return {
    name: String(cfg.appName),
    url: String(cfg.siteUrl),
    indexable: cfg.indexable === true,
    defaultLocale: String(cfg.defaultLocale),
  }
}

export async function sitePages(event: H3Event): Promise<SitePage[]> {
  const { url, defaultLocale } = siteConfig(event)
  const pages = new Map<string, SitePage>()

  for (const locale of LOCALES) {
    const prefix = locale.code === defaultLocale ? '' : `/${locale.code}`
    const rows = await queryCollection(event, locale.collection)
      .select('path', 'title', 'description', 'noindex', 'updated')
      .all()

    for (const row of rows) {
      if (row.noindex) continue
      const page = pages.get(row.path) ?? { path: row.path, updated: row.updated, versions: [] }
      page.versions.push({
        code: locale.code,
        language: locale.language,
        url: `${url}${`${prefix}${row.path}`.replace(/\/+$/, '') || '/'}`,
        title: row.title,
        description: row.description,
      })
      pages.set(row.path, page)
    }
  }

  // Home first, then alphabetical: a stable order keeps the files diffable.
  return [...pages.values()].sort((a, b) => (a.path === '/' ? -1 : b.path === '/' ? 1 : a.path.localeCompare(b.path)))
}

export const xmlEscape = (s: string) => s.replace(/[<>&'"]/g, c => `&#${c.charCodeAt(0)};`)
