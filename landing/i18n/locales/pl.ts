// The catalogue is the .json next door; this file only re-exports it.
//
// Registering the .json directly makes Vite's json plugin wrap the output of
// @nuxtjs/i18n's own transform in JSON.parse(), and this app - the one that
// renders on the server - dies at boot on "Unexpected token 'c'".
import messages from './pl.json'

export default messages
