import { useAuthStore } from '../stores/auth'

/**
 * The single entry point for every HTTP call. Nothing calls $fetch directly,
 * because four behaviours have to be identical everywhere:
 *
 *  1. **The access token lives in memory** and is attached here. It is never in
 *     localStorage - an XSS in any dependency could then steal a credential that
 *     outlives the tab (ADR-0018).
 *  2. **401 triggers exactly one silent refresh**, then a retry. Concurrent
 *     calls share that refresh rather than each starting their own, which would
 *     rotate the token N times and invalidate N-1 of them.
 *  3. **409 becomes a typed ConflictError** carrying both versions, so the
 *     conflict bar can offer reload / overwrite / diff instead of "your work is
 *     gone".
 *  4. **Cookies are always sent** (`credentials: 'include'`), because the
 *     refresh token is an httpOnly cookie on the API host.
 *  5. **413 becomes a typed PayloadTooLargeError** whose message is the
 *     user's sentence with the limit in it (`details.maxBytes`), so a page
 *     that toasts `error.message` says something a person can act on.
 */

export interface ApiErrorBody {
  error: {
    code: string
    message: string
    details?: Record<string, unknown>
    requestId?: string
  }
}

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly details?: Record<string, unknown>,
    /** Quote this when reporting a problem: it ties the UI to the server log. */
    readonly requestId?: string,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

/** Someone else changed the record first. Carries enough to offer a real choice. */
export class ConflictError extends ApiError {
  readonly yourVersion: number | string | null
  readonly currentVersion: number | string | null
  readonly current: Record<string, unknown> | null

  constructor(body: ApiErrorBody) {
    super(409, body.error.code, body.error.message, body.error.details, body.error.requestId)
    this.name = 'ConflictError'

    const d = (body.error.details ?? {}) as Record<string, unknown>
    this.yourVersion = (d.yourVersion as number | string | null) ?? null
    this.currentVersion = (d.currentVersion as number | string | null) ?? null
    this.current = (d.current as Record<string, unknown> | null) ?? null
  }
}

/**
 * The body was over the server's limit (413, `payload_too_large`). maxBytes
 * is the limit from the envelope, or null when something in front of the API
 * refused it without one; the message is already translated.
 */
export class PayloadTooLargeError extends ApiError {
  constructor(body: ApiErrorBody | null, message: string, readonly maxBytes: number | null) {
    super(413, body?.error.code ?? 'payload_too_large', message, body?.error.details, body?.error.requestId)
    this.name = 'PayloadTooLargeError'
  }
}

/** A byte limit as people read it: 25, 0.5, 1.5 - in MB. */
function megabytes(bytes: number): string {
  return String(Math.round((bytes / 1048576) * 10) / 10)
}

export interface ApiOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  body?: unknown
  query?: Record<string, string | number | boolean | undefined>
  headers?: Record<string, string>
  /** The version this write is based on. Sent as If-Match; a mismatch is a 409. */
  ifMatch?: number | string
  /** Set for the auth endpoints themselves, so a failed refresh cannot recurse. */
  skipAuthRetry?: boolean
}

/**
 * Shared across every caller in the tab, deliberately.
 *
 * Without it, five components hitting a stale token would each start a refresh;
 * the server rotates on use, so four of the five would present an
 * already-consumed token - which reuse detection correctly treats as theft and
 * revokes the whole session.
 */
let refreshInFlight: Promise<boolean> | null = null

export function useApi() {
  const config = useRuntimeConfig()
  const auth = useAuthStore()
  const base = String(config.public.apiBase ?? '')
  const { $i18n } = useNuxtApp()

  async function refreshOnce(): Promise<boolean> {
    refreshInFlight ??= (async () => {
      try {
        const res = await fetch(`${base}/api/auth/refresh`, {
          method: 'POST',
          credentials: 'include',
          headers: { 'Content-Type': 'application/json' },
        })
        // 204 is "no refresh cookie": nothing to retry with.
        if (res.status !== 200) return false

        const data = await res.json()
        auth.setSession(data)
        return true
      } catch {
        return false
      } finally {
        // Cleared only after everyone waiting has resolved, so the next 401
        // starts a genuinely new attempt.
        refreshInFlight = null
      }
    })()

    return refreshInFlight
  }

  async function request<T>(path: string, options: ApiOptions = {}): Promise<T> {
    const url = new URL(base + path)

    for (const [key, value] of Object.entries(options.query ?? {})) {
      if (value !== undefined) url.searchParams.set(key, String(value))
    }

    const send = async (): Promise<Response> => {
      const headers: Record<string, string> = {
        Accept: 'application/json',
        ...options.headers,
      }

      if (options.body !== undefined) headers['Content-Type'] = 'application/json'
      if (auth.token) headers.Authorization = `Bearer ${auth.token}`
      // Quoted, per RFC 7232. The server tolerates both forms, but sending a
      // well-formed validator is free.
      if (options.ifMatch !== undefined) headers['If-Match'] = `"${options.ifMatch}"`

      return fetch(url.toString(), {
        method: options.method ?? 'GET',
        headers,
        // The refresh token is an httpOnly cookie; without this it is never sent.
        credentials: 'include',
        body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
      })
    }

    let response = await send()

    if (response.status === 401 && !options.skipAuthRetry) {
      // Exactly one retry. A second 401 means the session is genuinely gone, and
      // looping would turn an expired login into a request storm.
      if (await refreshOnce()) {
        response = await send()
      } else {
        auth.clear()
      }
    }

    if (response.status === 204) return undefined as T
    if (response.ok) return (await response.json()) as T

    const body = await response.json().catch(() => null) as ApiErrorBody | null

    // Before the envelope check: a proxy in front of the API can refuse a
    // large body with no envelope, and the person still needs the sentence.
    if (response.status === 413) {
      const envelope = body?.error ? body : null
      const max = envelope?.error.details?.maxBytes
      const maxBytes = typeof max === 'number' && max > 0 ? max : null
      const message = maxBytes
        ? $i18n.t('error.payloadTooLarge', { limit: megabytes(maxBytes) })
        : $i18n.t('error.payloadTooLargeNoLimit')
      throw new PayloadTooLargeError(envelope, message, maxBytes)
    }

    if (!body?.error) {
      throw new ApiError(response.status, 'unexpected', `Request failed (${response.status}).`)
    }

    if (response.status === 409) throw new ConflictError(body)

    throw new ApiError(
      response.status,
      body.error.code,
      body.error.message,
      body.error.details,
      body.error.requestId,
    )
  }

  return {
    request,
    get: <T>(path: string, o: Omit<ApiOptions, 'method' | 'body'> = {}) =>
      request<T>(path, { ...o, method: 'GET' }),
    post: <T>(path: string, body?: unknown, o: Omit<ApiOptions, 'method' | 'body'> = {}) =>
      request<T>(path, { ...o, method: 'POST', body }),
    put: <T>(path: string, body?: unknown, o: Omit<ApiOptions, 'method' | 'body'> = {}) =>
      request<T>(path, { ...o, method: 'PUT', body }),
    patch: <T>(path: string, body?: unknown, o: Omit<ApiOptions, 'method' | 'body'> = {}) =>
      request<T>(path, { ...o, method: 'PATCH', body }),
    del: <T>(path: string, o: Omit<ApiOptions, 'method' | 'body'> = {}) =>
      request<T>(path, { ...o, method: 'DELETE' }),
  }
}
