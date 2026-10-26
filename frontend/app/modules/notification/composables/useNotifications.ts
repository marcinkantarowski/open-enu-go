import { ref } from 'vue'

/**
 * The feed, shared by the bell and the page.
 *
 * Module-scoped so both read one list: two independent fetches would show two
 * different unread counts on the same screen, and the one in the header is the
 * one people trust.
 */
export interface FeedItem {
  id: string
  type: string
  titleKey: string
  bodyKey: string
  context: Record<string, string | number | null>
  read: boolean
  createdAt: string
}

const items = ref<FeedItem[]>([])
const unread = ref(0)
const loaded = ref(false)

export function useNotifications() {
  const api = useApi()

  async function load() {
    try {
      const data = await api.get<{ items: FeedItem[], meta: { unread: number } }>('/api/notifications')
      items.value = data.items
      unread.value = data.meta.unread
    } catch {
      // A feed that cannot load must not blank the shell it sits in.
    } finally {
      loaded.value = true
    }
  }

  async function markRead(id: string) {
    await api.post(`/api/notifications/${id}/read`)
    await load()
  }

  async function markAllRead() {
    await api.post('/api/notifications/read-all')
    await load()
  }

  /**
   * Translation arguments come back as `{url: …}` and vue-i18n wants named
   * placeholders, so the message key does the rest. The row never stores a
   * sentence - that is what makes a feed readable in a language it was not
   * written in.
   */
  function argumentsFor(item: FeedItem): Record<string, string> {
    return Object.fromEntries(Object.entries(item.context).map(([k, v]) => [k, String(v ?? '')]))
  }

  return { items, unread, loaded, load, markRead, markAllRead, argumentsFor }
}
