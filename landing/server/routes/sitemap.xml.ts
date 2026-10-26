/**
 * Every page in every language, each entry naming its translations. hreflang in
 * the sitemap and hreflang in the page head say the same thing on purpose: a
 * search engine trusts the pair more than either alone.
 */
export default defineEventHandler(async (event) => {
  const pages = await sitePages(event)
  const { defaultLocale } = siteConfig(event)
  setHeader(event, 'Content-Type', 'application/xml; charset=utf-8')

  const urls = pages.flatMap(page => page.versions.map((version) => {
    const fallback = page.versions.find(v => v.code === defaultLocale) ?? version
    const alternates = [
      ...page.versions.map(v => `    <xhtml:link rel="alternate" hreflang="${v.language}" href="${xmlEscape(v.url)}"/>`),
      `    <xhtml:link rel="alternate" hreflang="x-default" href="${xmlEscape(fallback.url)}"/>`,
    ]
    const lastmod = page.updated ? [`    <lastmod>${xmlEscape(page.updated)}</lastmod>`] : []
    return ['  <url>', `    <loc>${xmlEscape(version.url)}</loc>`, ...lastmod, ...alternates, '  </url>'].join('\n')
  }))

  return [
    '<?xml version="1.0" encoding="UTF-8"?>',
    '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">',
    ...urls,
    '</urlset>',
    '',
  ].join('\n')
})
