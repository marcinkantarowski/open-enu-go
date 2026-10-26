import { ref, shallowRef } from 'vue'
import { ConflictError } from './useApi'

/**
 * Turns a 409 into a choice.
 *
 * The default behaviour of most apps on a conflict is to show "someone else
 * changed this, please reload" - which discards the user's work and tells them
 * nothing about what differs. The server sends both versions and the current
 * record precisely so the UI can do better, and this is where that gets used.
 */
export function useConflict() {
  const conflict = shallowRef<ConflictError | null>(null)
  const resolving = ref(false)

  /**
   * Run a write, capturing a conflict instead of throwing it.
   *
   * Anything that is not a conflict still throws: a 500 is not something the
   * user can resolve by choosing, and swallowing it here would hide it.
   */
  async function attempt<T>(write: () => Promise<T>): Promise<T | null> {
    conflict.value = null

    try {
      return await write()
    } catch (error) {
      if (error instanceof ConflictError) {
        conflict.value = error
        return null
      }
      throw error
    }
  }

  /** Take the server's version, discarding the local edit. */
  async function reload<T>(fetchCurrent: () => Promise<T>): Promise<T> {
    resolving.value = true
    try {
      const fresh = await fetchCurrent()
      conflict.value = null
      return fresh
    } finally {
      resolving.value = false
    }
  }

  /**
   * Retry the write against the version that now exists.
   *
   * Deliberately explicit: overwriting someone's change is a decision, and
   * offering it without the diff above it would make it the easy default.
   */
  async function overwrite<T>(write: (version: number | string) => Promise<T>): Promise<T | null> {
    const current = conflict.value?.currentVersion
    if (current === null || current === undefined) return null

    resolving.value = true
    try {
      const result = await write(current)
      conflict.value = null
      return result
    } finally {
      resolving.value = false
    }
  }

  function dismiss() {
    conflict.value = null
  }

  return { conflict, resolving, attempt, reload, overwrite, dismiss }
}
