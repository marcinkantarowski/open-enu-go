import { getCurrentScope, onScopeDispose, ref } from 'vue'

/**
 * Follow a long-running job to completion.
 *
 * Polls, and listens. The two are not redundant: SSE gives an immediate update
 * when the worker reports, and the poll is the floor that keeps the UI honest if
 * the connection drops or the job finished before the listener attached.
 */
export interface ProgressJob {
  id: string
  kind: string
  label: string | null
  total: number
  done: number
  percent: number
  status: 'running' | 'done' | 'failed'
  result: Record<string, unknown>
  failureReason: string | null
}

export function useProgress(jobId: string, options: { pollMs?: number } = {}) {
  const api = useApi()
  const job = ref<ProgressJob | null>(null)
  const error = ref<string | null>(null)

  let timer: ReturnType<typeof setInterval> | null = null

  const stop = () => {
    if (timer !== null) {
      clearInterval(timer)
      timer = null
    }
  }

  async function poll() {
    try {
      job.value = await api.get<ProgressJob>(`/api/progress/${jobId}`)

      // Stop as soon as there is nothing left to learn. A finished job polled
      // forever is a request every few seconds, per open tab, indefinitely.
      if (job.value.status !== 'running') stop()
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Could not read progress.'
      stop()
    }
  }

  void poll()
  timer = setInterval(() => void poll(), options.pollMs ?? 2000)

  if (getCurrentScope()) onScopeDispose(stop)

  return { job, error, stop, refresh: poll }
}
