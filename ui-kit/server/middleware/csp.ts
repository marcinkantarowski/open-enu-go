/**
 * Content-Security-Policy on every response of the three apps.
 *
 * Defence in depth for markup the app renders as HTML (the document preview
 * is `v-html`): even if something injected gets to run, it cannot load a
 * script from elsewhere or send what it reads to another origin. The two
 * directives that matter are `connect-src`, which names the only origins a
 * page may talk to, and `img-src`, which closes the `<img src=…?data>`
 * channel to a foreign host.
 *
 * Built per request from the runtime config, so the API and Mercure origins
 * are those of the deployment and no domain is written here. The landing has
 * neither, and talks to nothing but itself.
 *
 * `script-src 'unsafe-inline'`: Nuxt writes the runtime config as an inline
 * `<script>window.__NUXT__…</script>` into every page, and its contents vary
 * with the deployment, so neither a fixed hash nor a nonce (the two SPAs are
 * static shells) can stand in for it. `'wasm-unsafe-eval'`, on the landing
 * only: @nuxt/content queries its SQLite database as WebAssembly in the
 * browser on client-side navigation. Neither allows a script from another origin.
 */
const origin = (url: unknown) => {
  try {
    return url ? new URL(String(url)).origin : ''
  } catch {
    return ''
  }
}

export default defineEventHandler((event) => {
  const pub = useRuntimeConfig(event).public as Record<string, unknown>
  const api = origin(pub.apiBase)
  const mercure = origin(pub.mercureUrl)
  const connect = ["'self'", api, mercure].filter((v, i, all) => v && all.indexOf(v) === i)

  setHeader(event, 'Content-Security-Policy', [
    "default-src 'self'",
    // The landing is the app without an API; only it runs @nuxt/content.
    `script-src 'self' 'unsafe-inline'${api ? '' : " 'wasm-unsafe-eval'"}`,
    "style-src 'self' 'unsafe-inline'",
    "img-src 'self' data: blob:",
    "font-src 'self' data:",
    `connect-src ${connect.join(' ')}`,
    "frame-ancestors 'none'",
    "object-src 'none'",
    "base-uri 'self'",
    "form-action 'self'",
  ].join('; '))
})
