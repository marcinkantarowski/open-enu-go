import { defineNavigation } from '@ui-kit/composables/defineNavigation'

export default defineNavigation([
  { label: 'settings.title', to: '/settings', permission: 'settings.view', order: 90 },
])
