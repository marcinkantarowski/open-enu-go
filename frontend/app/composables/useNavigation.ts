import type { NavigationItem, NavigationSection, SettingsTab } from '@ui-kit/composables/defineNavigation'
import { useAuthStore } from '@ui-kit/stores/auth'

/**
 * The menu, assembled from the modules rather than written down.
 *
 * Each module layer exports `navigation.ts`; this glob finds them at build time.
 * The alternative - a central array listing every module - is the file two
 * feature branches both edit, and the file someone forgets, producing a page
 * that exists but cannot be reached.
 *
 * `eager: true` because the menu is needed on first paint; lazy chunks here
 * would make the sidebar pop in after the content.
 */
const navigationModules = import.meta.glob<{ default: NavigationItem[] }>(
  '../modules/*/navigation.ts',
  { eager: true },
)

const settingsModules = import.meta.glob<{ default: SettingsTab[] }>(
  '../modules/*/settings-tabs.ts',
  { eager: true },
)

export function useNavigation() {
  const auth = useAuthStore()
  const { t } = useI18n()
  const { public: cfg } = useRuntimeConfig()
  const capabilities = useState<string[]>('navigation.capabilities', () => [])

  const translate = <T extends { label: string }>(item: T): T => ({ ...item, label: t(item.label) })

  const items = computed<NavigationItem[]>(() =>
    Object.values(navigationModules)
      .flatMap(mod => mod.default ?? [])
      // Hidden, not disabled: an entry someone cannot use is noise, and the
      // server refuses the request regardless of what renders here.
      .filter(item => !item.permission || auth.can(item.permission))
      .filter(item => !item.minWorkspaces
        || auth.memberships.filter(m => m.status === 'active').length >= item.minWorkspaces)
      .filter(item => !item.capability || capabilities.value.includes(item.capability))
      // The reference module is the platform showing itself off. A product
      // keeps the module - tests stand on it - and stops advertising it.
      .filter(item => !item.demo || cfg.demoContent)
      .sort((a, b) => (a.order ?? 100) - (b.order ?? 100))
      .map(translate),
  )

  const sections = computed<NavigationSection[]>(() => {
    const out: NavigationSection[] = []

    for (const item of items.value.filter(entry => !entry.topbar)) {
      const group = item.group ?? ''
      let section = out.find(entry => entry.group === group)

      if (!section) {
        section = { group, label: group ? t(group) : '', items: [] }
        out.push(section)
      }
      section.items.push(item)
    }

    return out
  })

  const topbar = computed(() => items.value.filter(item => item.topbar))

  const settingsTabs = computed<SettingsTab[]>(() =>
    Object.values(settingsModules)
      .flatMap(mod => mod.default ?? [])
      .filter(tab => !tab.permission || auth.can(tab.permission))
      .sort((a, b) => (a.order ?? 100) - (b.order ?? 100))
      .map(translate),
  )

  return { sections, topbar, settingsTabs }
}
