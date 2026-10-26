/**
 * The page-meta keys this app's middleware reads.
 *
 * Declared rather than left as `any`: a typo in `definePageMeta({ publik: true })`
 * would otherwise be a silently unauthenticated page.
 */
declare module 'vue-router' {
  interface RouteMeta {
    /** Reachable without a session. The exception, stated at the page. */
    public?: boolean
    /** Required permission, enforced by the `permission` middleware. */
    permission?: string
  }
}

export {}
