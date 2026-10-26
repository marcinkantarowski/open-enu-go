import { defineSettingsTab } from '@ui-kit/composables/defineNavigation'

// The tab bar is assembled the same way the menu is: every module that owns a
// settings screen registers its own tab, so adding one never means editing a
// page that belongs to another module (.ai/platform/docs/frontend-conventions.md).
export default defineSettingsTab([
  { label: 'settings.workspace', to: '/settings', permission: 'settings.view', order: 20 },
])
