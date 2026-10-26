import { useAuthStore } from '@ui-kit/stores/auth'

/**
 * Route-level permission check.
 *
 *     definePageMeta({ middleware: 'permission', permission: 'api_key.view' })
 *
 * A convenience, not a control: the server checks the same permission on every
 * request it serves. What this buys is a 403 page instead of a screen full of
 * failed requests.
 */
export default defineNuxtRouteMiddleware((to) => {
  const required = to.meta.permission

  if (typeof required !== 'string') return

  const auth = useAuthStore()

  if (!auth.can(required)) {
    return abortNavigation({ statusCode: 403, statusMessage: 'Forbidden' })
  }
})
