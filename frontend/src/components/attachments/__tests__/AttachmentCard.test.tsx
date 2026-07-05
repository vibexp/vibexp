import { fireEvent, render, screen, waitFor } from '@testing-library/react'

import { AttachmentCard } from '@/components/attachments/AttachmentCard'
import type { Attachment } from '@/services/attachmentService'

function makeAttachment(overrides: Partial<Attachment> = {}): Attachment {
  return {
    id: 'att-1',
    team_id: 'team-1',
    owner_type: 'artifact',
    owner_id: 'artifact-1',
    file_name: 'spec.pdf',
    content_type: 'application/pdf',
    size_bytes: 2048,
    created_at: '2024-01-15T10:30:00Z',
    ...overrides,
  }
}

function sizedFile(name: string, size: number, type = 'text/plain'): File {
  const file = new File(['x'], name, { type })
  Object.defineProperty(file, 'size', { value: size })
  return file
}

function renderCard(
  props: Partial<React.ComponentProps<typeof AttachmentCard>> = {}
) {
  const onUpload = jest.fn().mockResolvedValue(undefined)
  const onDelete = jest.fn().mockResolvedValue(undefined)
  const onDownload = jest.fn()
  render(
    <AttachmentCard
      attachments={props.attachments ?? []}
      onUpload={onUpload}
      onDelete={onDelete}
      onDownload={onDownload}
      {...props}
    />
  )
  return { onUpload, onDelete, onDownload }
}

describe('AttachmentCard', () => {
  it('renders the empty state with no attachments', () => {
    renderCard()
    expect(screen.getByText('No attachments yet.')).toBeInTheDocument()
  })

  it('shows the quota meter and file count', () => {
    renderCard({
      attachments: [makeAttachment({ size_bytes: 2 * 1024 * 1024 })],
    })
    expect(screen.getByText(/2\.0 MB of 10\.0 MB used/i)).toBeInTheDocument()
    expect(screen.getByText('1 file')).toBeInTheDocument()
  })

  it('derives a kind label from the content type', () => {
    renderCard({
      attachments: [
        makeAttachment({ file_name: 'photo.png', content_type: 'image/png' }),
      ],
    })
    expect(screen.getByText(/^Image ·/)).toBeInTheDocument()
  })

  it('lists attachments and triggers download', () => {
    const { onDownload } = renderCard({
      attachments: [makeAttachment({ file_name: 'notes.txt' })],
    })
    expect(screen.getByText('notes.txt')).toBeInTheDocument()
    fireEvent.click(screen.getByLabelText('Download notes.txt'))
    expect(onDownload).toHaveBeenCalledTimes(1)
  })

  it('deletes an attachment', async () => {
    const { onDelete } = renderCard({
      attachments: [makeAttachment({ file_name: 'notes.txt' })],
    })
    fireEvent.click(screen.getByLabelText('Delete notes.txt'))
    await waitFor(() => {
      expect(onDelete).toHaveBeenCalledTimes(1)
    })
  })

  it('uploads a valid file', async () => {
    const { onUpload } = renderCard()
    const input = screen.getByLabelText('Upload attachment')
    fireEvent.change(input, {
      target: { files: [sizedFile('notes.txt', 100)] },
    })
    await waitFor(() => {
      expect(onUpload).toHaveBeenCalledTimes(1)
    })
  })

  it('rejects a disallowed file type without uploading', async () => {
    const { onUpload } = renderCard()
    const input = screen.getByLabelText('Upload attachment')
    fireEvent.change(input, {
      target: {
        files: [sizedFile('malware.exe', 100, 'application/octet-stream')],
      },
    })
    await waitFor(() => {
      expect(screen.getByText(/not allowed/i)).toBeInTheDocument()
    })
    expect(onUpload).not.toHaveBeenCalled()
  })

  it('rejects a file over the per-file size limit', async () => {
    const { onUpload } = renderCard()
    const input = screen.getByLabelText('Upload attachment')
    fireEvent.change(input, {
      target: { files: [sizedFile('big.txt', 6 * 1024 * 1024)] },
    })
    await waitFor(() => {
      expect(screen.getByText(/per-file limit/i)).toBeInTheDocument()
    })
    expect(onUpload).not.toHaveBeenCalled()
  })

  it('rejects a file that would exceed the total size limit', async () => {
    const { onUpload } = renderCard({
      attachments: [makeAttachment({ size_bytes: 9 * 1024 * 1024 })],
    })
    const input = screen.getByLabelText('Upload attachment')
    fireEvent.change(input, {
      target: { files: [sizedFile('more.txt', 2 * 1024 * 1024)] },
    })
    await waitFor(() => {
      expect(screen.getByText(/total limit/i)).toBeInTheDocument()
    })
    expect(onUpload).not.toHaveBeenCalled()
  })

  it('disables the Add file button at capacity', () => {
    renderCard({
      attachments: [makeAttachment({ size_bytes: 10 * 1024 * 1024 })],
    })
    expect(screen.getByTestId('attachment-add-button')).toBeDisabled()
  })
})
