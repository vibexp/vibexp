import { render, screen, waitFor } from '@testing-library/react'

import { ResourceAttachments } from '@/components/attachments/ResourceAttachments'
import type { Attachment } from '@/services/attachmentService'

const mockList = vi.hoisted(() => vi.fn())
const mockUpload = vi.hoisted(() => vi.fn())
const mockRemove = vi.hoisted(() => vi.fn())
const mockDownload = vi.hoisted(() => vi.fn())

vi.mock('@/services/attachmentService', () => ({
  attachmentService: {
    list: (...args: unknown[]) => mockList(...args),
    upload: (...args: unknown[]) => mockUpload(...args),
    remove: (...args: unknown[]) => mockRemove(...args),
    download: (...args: unknown[]) => mockDownload(...args),
  },
}))

const showSuccess = vi.fn()
const showError = vi.fn()
vi.mock('@/hooks', () => ({
  useAlerts: () => ({ showSuccess, showError }),
}))

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

describe('ResourceAttachments', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('loads attachments for the given owner on mount', async () => {
    mockList.mockResolvedValue({
      attachments: [makeAttachment()],
      total_count: 1,
      total_size_bytes: 2048,
    })

    render(
      <ResourceAttachments
        teamId="team-1"
        ownerType="artifact"
        ownerId="artifact-1"
      />
    )

    await waitFor(() => {
      expect(mockList).toHaveBeenCalledWith('team-1', 'artifact', 'artifact-1')
    })
    expect(await screen.findByText('spec.pdf')).toBeInTheDocument()
  })

  it('surfaces a load error via the alerts hook', async () => {
    mockList.mockRejectedValue(new Error('boom'))

    render(
      <ResourceAttachments
        teamId="team-1"
        ownerType="artifact"
        ownerId="artifact-1"
      />
    )

    await waitFor(() => {
      expect(showError).toHaveBeenCalled()
    })
  })
})
