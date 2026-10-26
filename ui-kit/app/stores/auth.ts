import { defineStore } from 'pinia'

/**
 * The session, held in memory only.
 *
 * There is deliberately no persistence here - no localStorage, no cookie this
 * code can read. A reload costs one silent refresh against the httpOnly cookie,
 * which is the trade ADR-0018 makes: an XSS can act while the tab is open, but
 * cannot steal a credential that outlives it.
 */
export interface Membership {
  id: string
  tenantId: string
  role: string
  status: string
  /** Resolved through the Tenant module's contract - Membership holds only an id. */
  tenantName: string | null
  tenantSlug: string | null
}

export interface SessionUser {
  id: string
  email: string
  displayName: string | null
  status: string
  locale: string | null
  hasAvatar: boolean
}

export interface Session {
  token: string
  expiresIn: number
  tenantId: string | null
  user: SessionUser
  memberships: Membership[]
  role: string | null
  /**
   * What this role may do, as the server computes it.
   *
   * Advisory: it decides which controls render, never which requests succeed.
   * Sent rather than derived here because the rules live in one place on the
   * server, and a second copy in the client would drift in the worst
   * direction - a button that renders and then 403s.
   */
  permissions: string[]
  /** Present only for an impersonated session; drives the banner. */
  impersonating?: boolean
  operator?: { id: string, displayName: string | null }
}

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: null as string | null,
    tenantId: null as string | null,
    user: null as SessionUser | null,
    memberships: [] as Membership[],
    role: null as string | null,
    permissions: [] as string[],
    impersonating: false,
    operator: null as { id: string, displayName: string | null } | null,
    /** False until the first refresh attempt settles, so guards do not bounce. */
    ready: false,
  }),

  getters: {
    isAuthenticated: state => state.token !== null,

    currentMembership: (state): Membership | null =>
      state.memberships.find(m => m.tenantId === state.tenantId) ?? null,

    /** A switcher is noise for the overwhelming majority who belong to one. */
    canSwitchTenant: state => state.memberships.filter(m => m.status === 'active').length > 1,

    tenantName(): string | null {
      return this.currentMembership?.tenantName ?? null
    },

    /**
     * Whether to RENDER a control. Never whether to allow an action - the
     * server decides that on every request, from the same rules.
     */
    can: state => (permission: string): boolean => state.permissions.includes(permission),
  },

  actions: {
    setSession(session: Session) {
      this.token = session.token
      this.tenantId = session.tenantId
      this.user = session.user
      this.memberships = session.memberships ?? []
      this.role = session.role ?? null
      this.permissions = session.permissions ?? []
      this.impersonating = session.impersonating === true
      this.operator = session.operator ?? null
      this.ready = true
    },

    clear() {
      this.$reset()
      this.ready = true
    },
  },
})
