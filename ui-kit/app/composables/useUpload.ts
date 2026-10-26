import { ref } from 'vue'
import { useAuthStore } from '../stores/auth'

/**
 * File upload with real progress.
 *
 * XMLHttpRequest rather than fetch, for one reason: fetch has no upload progress
 * event. On a 20 MB file over a phone connection, a spinner with no percentage
 * is indistinguishable from a hang, and people cancel and retry - which makes it
 * worse.
 */
export interface UploadedFile {
  id: string
  filename: string
  contentType: string
  size: number
  ownerType: string | null
  ownerId: string | null
  createdAt: string
  /**
   * Only on endpoints that sign one. Upload returns metadata; the signed URL
   * comes from GET /api/attachments/{id}, freshly signed and short-lived,
   * because a storage URL that works forever is a credential.
   */
  url?: string
}

export function useUpload() {
  const config = useRuntimeConfig()
  const auth = useAuthStore()

  const progress = ref(0)
  const uploading = ref(false)
  const error = ref<string | null>(null)

  function upload(file: File, owner?: { type: string, id: string }): Promise<UploadedFile> {
    uploading.value = true
    progress.value = 0
    error.value = null

    return new Promise((resolve, reject) => {
      const form = new FormData()
      form.append('file', file)

      if (owner) {
        form.append('ownerType', owner.type)
        form.append('ownerId', owner.id)
      }

      const xhr = new XMLHttpRequest()
      xhr.open('POST', `${String(config.public.apiBase)}/api/attachments`)
      // Cookies for the session, header for the access token - the same
      // combination every other call uses.
      xhr.withCredentials = true

      if (auth.token) xhr.setRequestHeader('Authorization', `Bearer ${auth.token}`)
      // Content-Type is deliberately NOT set: the browser must add the multipart
      // boundary, and setting it by hand produces a body the server cannot parse.

      xhr.upload.onprogress = (event) => {
        if (event.lengthComputable) progress.value = Math.round((event.loaded / event.total) * 100)
      }

      xhr.onload = () => {
        uploading.value = false

        if (xhr.status >= 200 && xhr.status < 300) {
          resolve(JSON.parse(xhr.responseText) as UploadedFile)
          return
        }

        const message = (() => {
          try {
            return JSON.parse(xhr.responseText)?.error?.message ?? 'Upload failed.'
          } catch {
            return 'Upload failed.'
          }
        })()

        error.value = message
        reject(new Error(message))
      }

      xhr.onerror = () => {
        uploading.value = false
        error.value = 'The upload could not be completed.'
        reject(new Error(error.value))
      }

      xhr.send(form)
    })
  }

  return { upload, progress, uploading, error }
}
