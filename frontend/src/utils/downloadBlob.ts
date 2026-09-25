/**
 * Saves a `Blob` under `filename` through a temporary `<a download>` link, then
 * releases the object URL so the payload is not kept alive for the page's
 * lifetime.
 */
export function downloadBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}
