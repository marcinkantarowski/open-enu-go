/**
 * Anywhere but production, every response says "do not index" in a header -
 * pages, the sitemap, files in public/, all of it. robots.txt asks crawlers not
 * to come; this tells the ones that came anyway.
 */
export default defineEventHandler((event) => {
  if (!siteConfig(event).indexable) {
    setHeader(event, 'X-Robots-Tag', 'noindex, nofollow')
  }
})
