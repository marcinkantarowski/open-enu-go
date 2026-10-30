import { useAuthStore, type Session } from '@ui-kit/stores/auth'

/**
 * Operator sessions, which differ from tenant sessions in three ways.
 *
 *  1. A different endpoint and a different token audience (`manager`), so an
 *     operator token is refused on `/api` and a tenant token here (ADR-0019).
 *  2. **No refresh, by design.** The backend issues no cookie: a lapsed session
 *     means logging in again, which is a reasonable thing to ask of someone
 *     whose account reaches every tenant's data. The practical consequence is
 *     that a page reload signs the operator out - deliberate, not a bug.
 *  3. No tenant and no memberships. An operator is not a member of anything;
 *     authority comes from the realm, not from a role inside a workspace.
 */
export interface OperatorSession {
  token: string
  expiresIn: number
  manager: { id: string, email: string, displayName: string | null, status: string }
}

/** Ends the session when its token does, so nobody looks at a console that can no longer read. */
let expiry: ReturnType<typeof setTimeout> | undefined

export function useOperator() {
  const api = useApi()
  const auth = useAuthStore()

  async function login(email: string, password: string) {
    const session = await api.post<OperatorSession>(
      '/api/manager/login',
      { email, password },
      { skipAuthRetry: true },
    )

    // Adapted onto the shared store rather than duplicating one: the shell, the
    // API client and the toast host all read from it.
    auth.setSession({
      token: session.token,
      expiresIn: session.expiresIn,
      tenantId: null,
      user: { ...session.manager, locale: null, hasAvatar: false },
      memberships: [],
      role: 'platform_manager',
      permissions: [],
    } satisfies Session)

    clearTimeout(expiry)
    expiry = setTimeout(() => auth.clear(), session.expiresIn * 1000)

    return session
  }

  function logout() {
    // Nothing server-side to end: there is no refresh token to revoke.
    clearTimeout(expiry)
    auth.clear()
  }

  return { login, logout, auth }
}
