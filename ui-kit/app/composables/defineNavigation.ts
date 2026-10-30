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
  order?: number
  /**
   * Part of the platform's own demonstration, not of a product: shown only
   * while `DEMO_CONTENT` is on. The page behind it stays reachable by URL -
   * the platform's browser tests go there - it is merely not offered.
   */
  demo?: boolean
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
