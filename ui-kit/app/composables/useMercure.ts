import { computed, getCurrentScope, onScopeDispose, ref } from 'vue'
import { ApiError } from './useApi'

/**
 * One SSE connection per tab, shared by everything that listens.
 *
 * EventSource cannot set headers, so the subscriber credential is a cookie the
 * API issues - which is why connecting starts with a POST rather than simply
 * opening the stream.
 *
 * The connection is module-scoped rather than per-component: a page with six
 * live widgets would otherwise open six streams, and browsers cap concurrent
 * connections per host at around six.
 *
 * It stays open while anything wants it: the shell `hold()`s it for as long as
 * the signed-in layout is mounted, and every `subscribe()` wants it until it
 * stops. When nothing does, it closes and the status is `idle` - which is not
 * a fault, and the indicator shows nothing for it.
 */
type Listener = (payload: RealtimeMessage) => void

export interface RealtimeMessage {
  event: string
  subjectId: string
  occurredAt: string
  payload: Record<string, unknown>
}

/**
 * `idle`: nothing wants the stream, so none is open.
 * `connecting`: the first attempt is under way.
 * `live`: open, and events arrive.
 * `reconnecting`: it was wanted and is not open; another attempt is coming.
 */
export type RealtimeStatus = 'idle' | 'connecting' | 'live' | 'reconnecting'

const MAX_RETRY_MS = 30_000

let source: EventSource | null = null
let connecting: Promise<void> | null = null
let retryTimer: ReturnType<typeof setTimeout> | null = null
let attempt = 0
let holds = 0
const listeners = new Set<Listener>()
const status = ref<RealtimeStatus>('idle')
const connected = computed(() => status.value === 'live')

const wanted = () => holds > 0 || listeners.size > 0

function close() {
  if (retryTimer) clearTimeout(retryTimer)
  retryTimer = null
  source?.close()
  source = null
  attempt = 0
  status.value = 'idle'
}

export function useMercure() {
  const config = useRuntimeConfig()
  const api = useApi()

  function open(): Promise<void> {
    if (import.meta.server) return Promise.resolve()
    if (source) return Promise.resolve()
    if (connecting) return connecting

    if (status.value === 'idle') status.value = 'connecting'

    connecting = (async () => {
      try {
        // Sets the httpOnly `__Secure-mercure_access_token` cookie, scoped to
        // this tenant's topics only - a subscriber token for `*` would receive
        // every tenant's updates.
        const { topic } = await api.post<{ topic: string }>('/api/realtime/token')
        // Released while the token was in flight: open nothing.
        if (!wanted()) return close()

        const url = new URL(String(config.public.mercureUrl))
        // `match` is Mercure 1.x's exact matcher; the hub refuses `topic`.
        url.searchParams.append('match', topic)

        const stream = new EventSource(url.toString(), { withCredentials: true })
        source = stream

        stream.onopen = () => {
          attempt = 0
          status.value = 'live'
        }

        stream.onmessage = (event) => {
          try {
            const message = JSON.parse(event.data) as RealtimeMessage
            for (const listener of listeners) listener(message)
          } catch {
            // A malformed frame must not take down the stream for everyone.
          }
        }

        stream.onerror = () => {
          if (source !== stream) return
          if (stream.readyState === EventSource.CLOSED) {
            // The hub refused the reconnect outright, and EventSource never
            // retries a refusal. The usual reason is the subscriber cookie:
            // it lives an hour, and a stream the hub drops after that comes
            // back without it. A fresh token is what fixes that.
            stream.close()
            source = null
            retry()
          } else {
            // A dropped connection: EventSource reconnects on its own, with
            // backoff. Closing here would replace that with nothing.
            status.value = 'reconnecting'
          }
        }
      } catch (error) {
        // No session, or no workspace to listen to: another token will not
        // change that, so stop rather than ask again every thirty seconds.
        // The next subscribe() - after a sign-in - starts afresh.
        if (error instanceof ApiError && (error.status === 401 || error.status === 403)) return close()
        // The API is down or unreachable: try again, with backoff.
        retry()
      } finally {
        connecting = null
      }
    })()

    return connecting
  }

  function retry() {
    if (!wanted()) return close()
    status.value = 'reconnecting'
    if (retryTimer) return
    const delay = Math.min(MAX_RETRY_MS, 1_000 * 2 ** attempt)
    attempt++
    retryTimer = setTimeout(() => {
      retryTimer = null
      if (wanted()) void open()
    }, delay)
  }

  /**
   * Keep the stream open while the caller's scope lives - the signed-in shell,
   * so the connection survives navigating between pages that do and do not
   * listen. Returns the release, which runs on its own when the scope ends.
   */
  function hold(): () => void {
    holds++
    void open()

    let released = false
    const release = () => {
      if (released) return
      released = true
      holds--
      if (!wanted()) close()
    }

    if (getCurrentScope()) onScopeDispose(release)

    return release
  }

  function subscribe(listener: Listener): () => void {
    listeners.add(listener)
    void open()

    const stop = () => {
      listeners.delete(listener)
      // Last one out closes the stream, unless the shell is holding it.
      if (!wanted()) close()
    }

    // Tied to the component's scope when there is one, so navigating away
    // cleans up without every caller remembering to.
    if (getCurrentScope()) onScopeDispose(stop)

    return stop
  }

  return { hold, subscribe, status, connected }
}
