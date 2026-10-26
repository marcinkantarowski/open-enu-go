import { ref } from 'vue'

/**
 * Feature flags, resolved once per session.
 *
 * Deliberately not per-check: a flag read on every render would put a request
 * behind every component. The server is the authority - a route behind a flag
 * returns 404 regardless of what the client believes - so this exists only
 * to avoid rendering a button that cannot work.
 */
const values = ref<Record<string, unknown>>({})
const loaded = ref(false)

export function useFlags() {
  const api = useApi()

  async function load(force = false) {
    if (loaded.value && !force) return

    try {
      const { items } = await api.get<{ items: Array<{ identifier: string, effectiveValue: unknown }> }>(
        '/api/settings',
      )

      values.value = Object.fromEntries(items.map(i => [i.identifier, i.effectiveValue]))
      loaded.value = true
    } catch {
      // A failure here must not blank the UI. Unknown flags fall back to the
      // caller's default, which is the same as a fresh session.
      loaded.value = true
    }
  }

  function enabled(identifier: string, fallback = false): boolean {
    const value = values.value[identifier]

    if (value === undefined) return fallback
    if (typeof value === 'boolean') return value
    if (typeof value === 'string') return ['1', 'true', 'yes', 'on'].includes(value.toLowerCase())
    if (typeof value === 'number') return value !== 0

    return fallback
  }

  return { load, enabled, values, loaded }
}
