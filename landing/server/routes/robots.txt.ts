/**
 * Generated, never a static file: a static "allow" cannot know which stage it
 * is on, and a staging server indexed next to production is the failure this
 * exists to prevent. `indexable` is true for APP_STAGE=production only.
 *
 * `User-agent: *` covers AI crawlers too. To refuse them while staying in
 * search, add groups for GPTBot, ClaudeBot, Google-Extended, PerplexityBot and
 * CCBot with `Disallow: /` - see .ai/platform/docs/landing.md.
 */
export default defineEventHandler((event) => {
  const { url, indexable } = siteConfig(event)
  setHeader(event, 'Content-Type', 'text/plain; charset=utf-8')

  return indexable
    ? `User-agent: *\nAllow: /\n\nSitemap: ${url}/sitemap.xml\n`
    : 'User-agent: *\nDisallow: /\n'
})
