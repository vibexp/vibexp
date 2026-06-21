import { toast as sonner } from 'sonner'

interface ToastOptions {
  description?: string
  duration?: number
}

/**
 * V2 toast helper backed by sonner. Use instead of v1 AlertContext when on /v2.
 * Kept intentionally small — mirrors the surface we actually need.
 */
export const toast = {
  success(message: string, options?: ToastOptions) {
    return sonner.success(message, options)
  },
  error(message: string, options?: ToastOptions) {
    return sonner.error(message, options)
  },
  info(message: string, options?: ToastOptions) {
    return sonner.info(message, options)
  },
  warning(message: string, options?: ToastOptions) {
    return sonner.warning(message, options)
  },
  message(message: string, options?: ToastOptions) {
    return sonner(message, options)
  },
}
