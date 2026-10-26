import { useMercure, type RealtimeMessage } from './useMercure'

/**
 * Listen for one domain event by name.
 *
 *     useAppEvent('example.project.created', ({ payload }) => refresh())
 *
 * The name is the wire contract the backend publishes - the same string the
 * event's `eventName()` returns, and the same one a webhook subscribes to.
 */
export function useAppEvent(eventName: string, handler: (message: RealtimeMessage) => void) {
  const { subscribe, connected } = useMercure()

  const stop = subscribe((message) => {
    if (message.event === eventName) handler(message)
  })

  return { stop, connected }
}
