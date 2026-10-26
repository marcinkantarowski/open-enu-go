import { defineNavigation } from '@ui-kit/composables/defineNavigation'

export default defineNavigation([
  // `demo`: this module is the platform's reference, not a product screen.
  // Offered in the menu only while DEMO_CONTENT is on; `/example` itself stays.
  { label: 'example.title', to: '/example', permission: 'example.view', order: 10, demo: true },
])
