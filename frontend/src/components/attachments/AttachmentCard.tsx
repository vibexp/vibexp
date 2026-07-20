import {
  Download,
  File as FileGenericIcon,
  FileText,
  Image as ImageIcon,
  Paperclip,
  Plus,
  Trash2,
} from 'lucide-react'
import type { ComponentType } from 'react'
import { useRef, useState } from 'react'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { PanelTitle } from '@/components/ui/panel-title'
import type { Attachment } from '@/services/attachmentService'
import { formatFileSize } from '@/utils/formatFileSize'

// Defaults mirror the backend limits and safe-type allowlist (issue #1623).
const DEFAULT_MAX_FILE_SIZE = 5 * 1024 * 1024
const DEFAULT_MAX_TOTAL_SIZE = 10 * 1024 * 1024
const DEFAULT_ALLOWED_EXTENSIONS = [
  '.png',
  '.jpg',
  '.jpeg',
  '.gif',
  '.webp',
  '.pdf',
  '.txt',
  '.md',
  '.csv',
  '.json',
  '.docx',
  '.xlsx',
  '.zip',
]

export interface AttachmentCardProps {
  attachments: Attachment[]
  loading?: boolean
  disabled?: boolean
  /** Header label. @default "Attachments" */
  title?: string
  /** Uploads one file; should resolve once the attachment is persisted. */
  onUpload: (file: File) => Promise<void>
  onDelete: (attachment: Attachment) => Promise<void>
  onDownload: (attachment: Attachment) => void | Promise<void>
  maxFileSize?: number
  maxTotalSize?: number
  allowedExtensions?: string[]
}

function fileExtension(name: string): string {
  const dot = name.lastIndexOf('.')
  return dot === -1 ? '' : name.slice(dot).toLowerCase()
}

/**
 * Maps a MIME type to the row glyph + a short kind label shown before the size.
 * Mirrors the design-system storage card's four glyph buckets; anything outside
 * image/pdf/text falls back to the generic file icon.
 */
function fileMeta(contentType: string): {
  Icon: ComponentType<{ className?: string }>
  kind: string
} {
  if (contentType.startsWith('image/'))
    return { Icon: ImageIcon, kind: 'Image' }
  if (contentType === 'application/pdf')
    return { Icon: FileGenericIcon, kind: 'PDF' }
  if (contentType.startsWith('text/')) return { Icon: FileText, kind: 'Text' }
  return { Icon: FileText, kind: 'File' }
}

/**
 * Storage-aware attachment card. A faithful port of the design-system
 * `AttachmentCardStorage` mockup, rebuilt with Tailwind + shadcn primitives so
 * it reads like the rest of the frontend while binding to the same tokens. A
 * quota meter sits under the header; row actions reveal on hover/focus.
 *
 * Upload/delete/download actions are supplied as props so any resource
 * (artifact today; memory/blueprint later) can reuse it via its own service.
 */
export function AttachmentCard({
  attachments,
  loading = false,
  disabled = false,
  title = 'Attachments',
  onUpload,
  onDelete,
  onDownload,
  maxFileSize = DEFAULT_MAX_FILE_SIZE,
  maxTotalSize = DEFAULT_MAX_TOTAL_SIZE,
  allowedExtensions = DEFAULT_ALLOWED_EXTENSIONS,
}: Readonly<AttachmentCardProps>) {
  const inputRef = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState(false)
  const [deletingId, setDeletingId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const usedBytes = attachments.reduce((sum, a) => sum + a.size_bytes, 0)
  const usedPercent = Math.max(
    0,
    Math.min(100, (usedBytes / maxTotalSize) * 100)
  )
  const atCapacity = usedBytes >= maxTotalSize
  const fileLabel = `${String(attachments.length)} file${attachments.length === 1 ? '' : 's'}`

  const validate = (file: File): string | null => {
    const ext = fileExtension(file.name)
    if (!allowedExtensions.includes(ext)) {
      return `File type ${ext || '(none)'} is not allowed.`
    }
    if (file.size > maxFileSize) {
      return `"${file.name}" exceeds the ${formatFileSize(maxFileSize)} per-file limit.`
    }
    if (usedBytes + file.size > maxTotalSize) {
      return `Adding "${file.name}" would exceed the ${formatFileSize(maxTotalSize)} total limit.`
    }
    return null
  }

  const handleFiles = async (files: FileList | null) => {
    if (!files || files.length === 0) return
    setError(null)
    const file = files[0]
    const validationError = validate(file)
    if (validationError) {
      setError(validationError)
      return
    }
    setBusy(true)
    try {
      await onUpload(file)
    } finally {
      setBusy(false)
      if (inputRef.current) inputRef.current.value = ''
    }
  }

  const handleDelete = async (attachment: Attachment) => {
    setDeletingId(attachment.id)
    try {
      await onDelete(attachment)
    } finally {
      setDeletingId(null)
    }
  }

  const renderAttachmentList = () => {
    if (attachments.length === 0) {
      return (
        <p className="text-muted-foreground px-5 py-4 text-sm">
          No attachments yet.
        </p>
      )
    }
    return (
      <ul className="divide-border m-0 list-none divide-y p-0">
        {attachments.map(attachment => {
          const { Icon, kind } = fileMeta(attachment.content_type)
          const isDeleting = deletingId === attachment.id
          // Prefer the skill-relative subpath (e.g. "scripts/helper.py") over the
          // basename so multi-file skill companions read as their in-repo path
          // (#345); plain attachments keep their file name.
          const label = attachment.relative_path ?? attachment.file_name
          return (
            <li
              key={attachment.id}
              className="group/item hover:bg-muted/55 flex items-center gap-3 px-5 py-3 transition-colors"
              data-testid="attachment-item"
            >
              <div className="bg-muted text-muted-foreground grid size-[38px] shrink-0 place-items-center rounded-md">
                <Icon className="size-[18px]" />
              </div>
              <div className="min-w-0 flex-1">
                <span
                  className="text-foreground block truncate text-sm font-medium"
                  title={label}
                >
                  {label}
                </span>
                <span className="text-muted-foreground mt-0.5 block text-xs tabular-nums">
                  {kind} · {formatFileSize(attachment.size_bytes)}
                </span>
              </div>
              <div className="flex shrink-0 items-center gap-0.5 opacity-0 transition-opacity group-focus-within/item:opacity-100 group-hover/item:opacity-100">
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="size-[30px]"
                  aria-label={`Download ${attachment.file_name}`}
                  title="Download"
                  onClick={() => {
                    void onDownload(attachment)
                  }}
                >
                  <Download className="size-[15px]" />
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="hover:bg-destructive/10 hover:text-destructive size-[30px]"
                  aria-label={`Delete ${attachment.file_name}`}
                  title="Delete"
                  disabled={isDeleting || disabled}
                  onClick={() => {
                    void handleDelete(attachment)
                  }}
                >
                  {isDeleting ? (
                    <LoadingSpinner size="sm" />
                  ) : (
                    <Trash2 className="size-[15px]" />
                  )}
                </Button>
              </div>
            </li>
          )
        })}
      </ul>
    )
  }

  return (
    <div
      className="bg-card text-card-foreground overflow-hidden rounded-lg border shadow-sm"
      data-testid="attachment-card"
    >
      {/* Header: paperclip + title (left), Add file button (right) */}
      <div className="flex items-center justify-between gap-3 px-5 pt-5 pb-4">
        <div className="flex min-w-0 items-center gap-2.5">
          <Paperclip className="text-muted-foreground size-[17px] shrink-0" />
          <PanelTitle>{title}</PanelTitle>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={disabled || busy || atCapacity}
          onClick={() => inputRef.current?.click()}
          data-testid="attachment-add-button"
        >
          {busy ? (
            <LoadingSpinner size="sm" />
          ) : (
            <Plus className="mr-1 size-3.5" />
          )}
          Add file
        </Button>
        <input
          ref={inputRef}
          type="file"
          className="hidden"
          accept={allowedExtensions.join(',')}
          aria-label="Upload attachment"
          onChange={e => {
            void handleFiles(e.target.files)
          }}
        />
      </div>

      {/* Quota meter */}
      <div className="px-5 pb-4">
        <div className="mb-[7px] flex items-baseline justify-between text-xs">
          <span className="text-muted-foreground tabular-nums">
            {formatFileSize(usedBytes)} of {formatFileSize(maxTotalSize)} used
          </span>
          <span className="text-muted-foreground">{fileLabel}</span>
        </div>
        <div className="bg-secondary h-[5px] overflow-hidden rounded-full">
          <div
            className="bg-primary h-full min-w-[6px] rounded-full"
            style={{ width: `${String(usedPercent)}%` }}
          />
        </div>
      </div>

      <div className="bg-border h-px" />

      {error && (
        <div className="px-5 pt-3">
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        </div>
      )}

      {loading ? (
        <div className="flex justify-center py-4">
          <LoadingSpinner size="sm" />
        </div>
      ) : (
        renderAttachmentList()
      )}
    </div>
  )
}
