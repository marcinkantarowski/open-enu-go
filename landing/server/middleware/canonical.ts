/**
 * One page, one URL. The site also answers on `www.` (the edge routes both hosts here)
 * and a path with a trailing slash renders the same page as the one without, so either
 * is sent on, permanently, to the address the canonical link names: the host of
 * `siteUrl`, no trailing slash, the query kept. The canonical link alone asks a search
 * engine to merge the two; the redirect stops the second one existing at all.
 */
export default defineEventHandler((event) => {
  if (event.method !== 'GET' && event.method !== 'HEAD') return

  const site = new URL(siteConfig(event).url)
  const [path, query] = splitPath(event.path)
  const host = getRequestHost(event, { xForwardedHost: true })
  const trimmed = path.length > 1 ? path.replace(/\/+$/, '') : path

  if (host === `www.${site.host}` || trimmed !== path) {
    return sendRedirect(event, `${site.origin}${trimmed || '/'}${query}`, 301)
  }
})

function splitPath(full: string): [string, string] {
  const i = full.indexOf('?')
  return i < 0 ? [full, ''] : [full.slice(0, i), full.slice(i)]
}
