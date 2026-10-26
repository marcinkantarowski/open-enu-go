import { defineNavigation } from '@ui-kit/composables/defineNavigation'

export default defineNavigation([
  { label: 'audit.title', to: '/audit', permission: 'audit.view', order: 80 },
])
