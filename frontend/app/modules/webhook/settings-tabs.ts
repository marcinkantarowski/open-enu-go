import { defineSettingsTab } from '@ui-kit/composables/defineNavigation'

export default defineSettingsTab([
  { label: 'webhook.title', to: '/settings/webhooks', permission: 'webhook.view', order: 40 },
])
