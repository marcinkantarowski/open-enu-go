import { defineSettingsTab } from '@ui-kit/composables/defineNavigation'

// Owned by Identity because Identity owns `/api/profile` - the tab lives with
// the module that serves it, not with the module that renders the tab bar.
export default defineSettingsTab([
  { label: 'identity.profile', to: '/settings/profile', order: 10 },
])
