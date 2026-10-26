import { defineStore } from 'pinia'

/**
 * Chrome state: the parts of the shell that are neither data nor session.
 *
 * Persisted in localStorage deliberately, and only this. Losing a collapsed
 * sidebar on reload is a small daily annoyance; the auth store next door
 * persists nothing at all, for reasons that do not apply here (ADR-0018).
 */
export const useUiStore = defineStore('ui', {
  state: () => ({
    sidebarOpen: true,
    locale: 'en',
  }),

  actions: {
    toggleSidebar() {
      this.sidebarOpen = !this.sidebarOpen
      this.persist()
    },

    setLocale(locale: string) {
      this.locale = locale
      this.persist()
    },

    restore() {
      if (import.meta.server) return

      try {
        const raw = localStorage.getItem('open_enu.ui')
        if (raw) Object.assign(this.$state, JSON.parse(raw))
      } catch {
        // A corrupt or blocked store is not worth a broken shell.
      }
    },

    persist() {
      if (import.meta.server) return

      try {
        localStorage.setItem('open_enu.ui', JSON.stringify(this.$state))
      } catch {
        // Private mode and "block site data" both throw here.
      }
    },
  },
})
