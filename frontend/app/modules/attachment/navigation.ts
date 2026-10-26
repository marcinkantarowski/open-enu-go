import { defineNavigation } from '@ui-kit/composables/defineNavigation'

// No page of its own: attachments belong to the records that own them, so this
// module contributes a component other modules place, not a screen people visit.
export default defineNavigation([])
