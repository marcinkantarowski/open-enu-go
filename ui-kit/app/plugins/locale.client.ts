/**
 * The language someone chose, remembered in the `open_enu_locale` cookie.
 *
 * Until they choose, the app speaks DEFAULT_LOCALE - never the browser's
 * language, which @nuxtjs/i18n would guess whenever it is asked to keep a
 * cookie, so that half of it is done here instead.
 *
 * Only for the two apps (`no_prefix`). The landing site's URL is its choice,
 * and switching a prefixed site's locale from a cookie would navigate it.
 */
const COOKIE = 'open_enu_locale'

export default defineNuxtPlugin({
  name: 'open-enu:locale',
  dependsOn: ['i18n:plugin'],

  async setup(nuxtApp) {
    const i18n = nuxtApp.$i18n
    if (i18n.strategy !== 'no_prefix') return

    // Not `Strict`: the landing site links into the app across subdomains, and
    // a strict cookie would be dropped on exactly that navigation.
    const chosen = useCookie<string | null>(COOKIE, { path: '/', maxAge: 60 * 60 * 24 * 365, sameSite: 'none', secure: true })

    const known = String(useRuntimeConfig().public.locales).split(',')
    if (chosen.value && known.includes(chosen.value) && chosen.value !== i18n.locale.value) {
      await i18n.setLocale(chosen.value as typeof i18n.locale.value)
    }

    nuxtApp.hook('i18n:localeSwitched', ({ newLocale }) => {
      chosen.value = newLocale
    })
  },
})
