import type { Component } from 'vue'

/**
 * The frontend extension surface (.ai/platform/docs/frontend-conventions.md).
 *
 * A module registers a widget for a named spot in someone else's page:
 *
 *     // frontend/app/modules/billing/injections.ts
 *     defineInjection('example.project.sidebar', InvoiceSummary)
 *
 * The page renders `<InjectionPoint name="example.project.sidebar" />` and never
 * learns that billing exists. That is the whole point: two modules extending the
 * same screen touch two different files, so adding a feature is not a merge
 * negotiation with whoever owns the page.
 */
export interface Injection {
  id: string
  name: string
  component: Component
  order: number
}

const registry = new Map<string, Injection[]>()

export function defineInjection(
  name: string,
  component: Component,
  options: { id?: string, order?: number } = {},
): void {
  const list = registry.get(name) ?? []

  const injection: Injection = {
    id: options.id ?? `${name}#${list.length}`,
    name,
    component,
    order: options.order ?? 100,
  }

  // Replace rather than append when the id repeats: HMR re-runs registration
  // files, and without this a saved edit doubles every widget on the page.
  const existing = list.findIndex(i => i.id === injection.id)
  if (existing >= 0) list.splice(existing, 1, injection)
  else list.push(injection)

  list.sort((a, b) => a.order - b.order)
  registry.set(name, list)
}

export function injectionsFor(name: string): Injection[] {
  return registry.get(name) ?? []
}
