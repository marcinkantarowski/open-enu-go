/**
 * A module contributes menu entries by exporting these from its layer.
 *
 * The shell merges and permission-filters them, so adding a feature never means
 * editing a central navigation file - which is the thing that makes two modules
 * conflict in a merge (.ai/platform/docs/frontend-conventions.md).
 */
export interface NavigationItem {
  /** Translation key, never a literal (ADR-0008). */
  label: string
  to: string
  icon?: string
  /** Hidden unless the session grants it. The server still enforces it. */
  permission?: string
  /** Hidden until the session has this many active workspaces: a view across them. */
  minWorkspaces?: number
  /**
   * Hidden until a module lists this name in the shared state
   * `navigation.capabilities`: what this workspace has switched on. Not a
   * guard - the server still decides what the page behind it may do.
   */
  capability?: string
  order?: number
  /**
   * Translation key of the heading this entry sits under. Headings follow the
   * lowest `order` among their entries. All start closed, one is open at a time
   * and the choice is not remembered; entries without one come as they are. While a capability is on, `<group>_<capability>` names
   * the heading instead, when that key exists.
   */
  group?: string
  /** A gear in the top bar instead of a line in the menu: where settings live. */
  topbar?: boolean
  /**
   * Part of the platform's own demonstration, not of a product: shown only
   * while `DEMO_CONTENT` is on. The page behind it stays reachable by URL -
   * the platform's browser tests go there - it is merely not offered.
   */
  demo?: boolean
}

/** The menu as the shell draws it: entries under their heading, in order. */
export interface NavigationSection {
  group: string
  label: string
  items: NavigationItem[]
}

export function defineNavigation(items: NavigationItem[]): NavigationItem[] {
  return items
}

export interface SettingsTab {
  label: string
  to: string
  permission?: string
  order?: number
}

export function defineSettingsTab(tabs: SettingsTab[]): SettingsTab[] {
  return tabs
}
