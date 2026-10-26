// The catalogue is the .json next door; this file only re-exports it.
//
// Registering the .json directly makes Vite's json plugin wrap the output of
// @nuxtjs/i18n's own transform in JSON.parse(), and the landing site - the one
// app that renders on the server - dies at boot on "Unexpected token 'c'".
// Pointing the loader at a .ts keeps the data in a file the parity check can
// read while taking it out of that plugin's path.
import messages from './en.json'

export default messages
