/**
 * Every app is out of search engines unless it says otherwise: `indexable` in its
 * public runtime config, which only the landing declares (true for APP_STAGE=production
 * and nowhere else). The two SPAs never declare it, so the tenant app and the operator
 * console - sign-in screens and all - are never indexed, on any stage.
 *
 * Not indexable means a header on every response (pages, files in public/, all of it)
 * and a robots.txt that turns crawlers away. The landing's own robots.txt route answers
 * only when it is indexable; here is the answer for every other case.
 */
const DISALLOW = 'User-agent: *\nDisallow: /\n'

export default defineEventHandler((event) => {
  if (useRuntimeConfig(event).public.indexable === true) return

  setHeader(event, 'X-Robots-Tag', 'noindex, nofollow')
  if (event.path === '/robots.txt') {
    setHeader(event, 'Content-Type', 'text/plain; charset=utf-8')
    return DISALLOW
  }
})
