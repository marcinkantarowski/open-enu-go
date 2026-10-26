import { defineNavigation } from '@ui-kit/composables/defineNavigation'

// No menu entry: this module's contribution to the tenant app is the workspace
// switcher in the header, not a page. Managing a workspace itself belongs to
// the operator console (`manager/`), not to the people inside it.
export default defineNavigation([])
