import { useCallback, useEffect, useState } from 'react'

import { AttachmentCard } from '@/components/attachments/AttachmentCard'
import { useAlerts } from '@/hooks'
import type { Attachment } from '@/services/attachmentService'
import { attachmentService } from '@/services/attachmentService'
import { getErrorMessage } from '@/utils/errorHandling'

interface ResourceAttachmentsProps {
  teamId: string
  ownerType: string
  ownerId: string
}

/**
 * Generic, resource-agnostic attachment manager. Owns attachment state and wires
 * the attachmentService into the reusable AttachmentCard for any owner type. A new
 * attachable resource renders this with its own ownerType/ownerId — no new
 * component or service code.
 */
export function ResourceAttachments({
  teamId,
  ownerType,
  ownerId,
}: ResourceAttachmentsProps) {
  const { showSuccess, showError } = useAlerts()
  const [attachments, setAttachments] = useState<Attachment[]>([])
  const [loading, setLoading] = useState(true)

  const refresh = useCallback(async () => {
    try {
      const res = await attachmentService.list(teamId, ownerType, ownerId)
      setAttachments(res.attachments)
    } catch (err) {
      showError(
        getErrorMessage(err, 'Failed to load attachments'),
        'Attachments'
      )
    } finally {
      setLoading(false)
    }
  }, [teamId, ownerType, ownerId, showError])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const handleUpload = async (file: File) => {
    try {
      await attachmentService.upload(teamId, ownerType, ownerId, file)
      showSuccess(`Uploaded ${file.name}`, 'Attachment added')
      await refresh()
    } catch (err) {
      showError(
        getErrorMessage(err, 'Failed to upload attachment'),
        'Upload failed'
      )
    }
  }

  const handleDelete = async (attachment: Attachment) => {
    try {
      await attachmentService.remove(teamId, attachment.id)
      showSuccess(`Deleted ${attachment.file_name}`, 'Attachment removed')
      await refresh()
    } catch (err) {
      showError(
        getErrorMessage(err, 'Failed to delete attachment'),
        'Delete failed'
      )
    }
  }

  const handleDownload = async (attachment: Attachment) => {
    try {
      const blob = await attachmentService.download(teamId, attachment.id)
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = attachment.file_name
      document.body.appendChild(link)
      link.click()
      link.remove()
      URL.revokeObjectURL(url)
    } catch (err) {
      showError(
        getErrorMessage(err, 'Failed to download attachment'),
        'Download failed'
      )
    }
  }

  return (
    <AttachmentCard
      attachments={attachments}
      loading={loading}
      onUpload={handleUpload}
      onDelete={handleDelete}
      onDownload={handleDownload}
    />
  )
}
