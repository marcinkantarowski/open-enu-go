import { useAuthStore, type Session } from '../stores/auth'

/**
 * Login, logout, and moving between tenants.
 *
 * Generic over the login endpoint so the manager app reuses all of it: the two
 * realms differ in which URL they post to and which token audience comes back,
 * and in nothing else worth duplicating.
 */
export function useAuth(options: { loginPath?: string } = {}) {
  const api = useApi()
  const auth = useAuthStore()
  const loginPath = options.loginPath ?? '/api/auth/login'

  async function login(email: string, password: string, tenantId?: string) {
    const session = await api.post<Session>(
      loginPath,
      { email, password, tenantId },
      // The login endpoint IS the auth endpoint; retrying it through a refresh
      // would be circular.
      { skipAuthRetry: true },
    )

    auth.setSession(session)
    return session
  }

  async function logout() {
    try {
      await api.post('/api/auth/logout', undefined, { skipAuthRetry: true })
    } finally {
      // Cleared whatever the server said. A logout that appears to fail leaves
      // someone believing they are still signed in.
      auth.clear()
    }
  }

  /**
   * Restore a session from the refresh cookie.
   *
   * Called once on boot. The access token lives in memory, so every reload
   * starts with none - this is what turns that into one silent round trip
   * instead of a login screen.
   */
  async function restore(): Promise<boolean> {
    try {
      // A 204 - no cookie, nothing to restore - arrives as undefined. It is
      // the answer on every first visit, which is why it is not a 401.
      const session = await api.post<Session | undefined>('/api/auth/refresh', undefined, { skipAuthRetry: true })
      if (!session) {
        auth.clear()
        return false
      }
      auth.setSession(session)
      return true
    } catch {
      auth.clear()
      return false
    }
  }

  /**
   * Switch tenant by re-issuing the session.
   *
   * Nothing stored changes: the tenant lives in the token (ADR-0010), so
   * switching is a new token and a fresh scope for every subsequent request.
   */
  async function switchTenant(tenantId: string) {
    const session = await api.post<Session>('/api/auth/switch-tenant', { tenantId }, { skipAuthRetry: true })
    auth.setSession(session)
    return session
  }

  return { login, logout, restore, switchTenant, auth }
}
