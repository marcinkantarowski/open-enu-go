import { ref } from 'vue'

export interface Toast {
  id: number
  message: string
  tone: 'info' | 'success' | 'error'
}

/** Shared across the app, so a toast raised in a composable reaches the shell. */
const toasts = ref<Toast[]>([])
let nextId = 1

export function useToast() {
  function push(message: string, tone: Toast['tone'] = 'info', ms = 5000) {
    const toast: Toast = { id: nextId++, message, tone }
    toasts.value = [...toasts.value, toast]

    // Errors stay until dismissed. Auto-hiding the one message someone needs to
    // read - or copy a request id from - is how a UI wastes their time.
    if (tone !== 'error') {
      setTimeout(() => dismiss(toast.id), ms)
    }

    return toast.id
  }

  function dismiss(id: number) {
    toasts.value = toasts.value.filter(t => t.id !== id)
  }

  return {
    toasts,
    dismiss,
    info: (m: string) => push(m, 'info'),
    success: (m: string) => push(m, 'success'),
    error: (m: string) => push(m, 'error'),
  }
}
