import { defineSettingsTab } from '@ui-kit/composables/defineNavigation'

export default defineSettingsTab([
  // `api_key.view` rather than `.manage`: an admin may see which keys exist
  // even though only an owner may mint one.
  { label: 'apiKey.title', to: '/settings/api-keys', permission: 'api_key.view', order: 30 },
])
