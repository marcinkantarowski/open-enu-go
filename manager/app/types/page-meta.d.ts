/**
 * The page-meta keys this console's middleware reads.
 *
 * Declared rather than left loose: a typo in `definePageMeta({ publik: true })`
 * would otherwise be a silently unauthenticated operator page.
 */
declare module 'vue-router' {
  interface RouteMeta {
    /** Reachable without a session. The exception, stated at the page. */
    public?: boolean
  }
}

export {}
