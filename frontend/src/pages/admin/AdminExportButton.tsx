import { Download, Loader2 } from 'lucide-react'
import { useRef, useState } from 'react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import type { AdminCsvExport } from '@/services/adminService'
import { ADMIN_EXPORT_ROW_CAP } from '@/services/adminService'
import { downloadBlob } from '@/utils/downloadBlob'
import { getErrorMessage } from '@/utils/errorHandling'

export interface AdminExportButtonProps {
  /** Runs the export for the list's current filters and sort. */
  onExport: () => Promise<AdminCsvExport>
  /** Set when there is nothing to export (the filtered total is 0). */
  disabled?: boolean
  /** Plural noun for the toasts, e.g. "users". */
  noun: string
}

/**
 * "Export CSV" for an admin list (#1150): downloads the whole filtered set and
 * says how much of it arrived. A capped export gets a warning with both numbers,
 * so an admin never mistakes a truncated file for the full population.
 */
export function AdminExportButton({
  onExport,
  disabled = false,
  noun,
}: Readonly<AdminExportButtonProps>) {
  const [exporting, setExporting] = useState(false)
  // State alone would let a fast double-click through before the re-render
  // that disables the button.
  const inFlight = useRef(false)

  const handleClick = () => {
    if (inFlight.current) return
    inFlight.current = true
    setExporting(true)
    onExport()
      .then(result => {
        downloadBlob(result.blob, result.filename)
        const total = result.totalCount.toLocaleString()
        if (result.truncated) {
          toast.warning(
            `Exported ${ADMIN_EXPORT_ROW_CAP.toLocaleString()} of ${total} ${noun} — the export is capped; narrow the filters to get the rest`
          )
        } else {
          toast.success(`Exported ${total} ${noun}`)
        }
      })
      .catch((err: unknown) => {
        toast.error(getErrorMessage(err, 'Export failed'))
      })
      .finally(() => {
        inFlight.current = false
        setExporting(false)
      })
  }

  return (
    <Button
      variant="outline"
      size="sm"
      onClick={handleClick}
      disabled={disabled || exporting}
    >
      {exporting ? (
        <Loader2 className="size-4 animate-spin" aria-hidden />
      ) : (
        <Download className="size-4" aria-hidden />
      )}
      Export CSV
    </Button>
  )
}
