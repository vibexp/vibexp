import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createRef } from 'react'

import type { Artifact } from '@/services/artifactService'

// Stub the async ProjectPicker to a button that selects a fixed project.
jest.mock('@/components/ProjectPicker', () => ({
  ProjectPicker: ({
    onChange,
  }: {
    onChange: (id: string | undefined) => void
  }) => (
    <button
      type="button"
      data-testid="artifact-project-select"
      onClick={() => {
        onChange('p1')
      }}
    >
      select project
    </button>
  ),
}))

jest.mock('@/hooks/useTypes', () => ({
  useTypes: () => ({
    types: [{ id: 't1', slug: 'general', name: 'General' }],
    isLoading: false,
  }),
}))

import { ArtifactForm, type ArtifactFormHandle } from '../ArtifactForm'

const baseArtifact: Artifact = {
  id: 'a-1',
  project_id: 'p1',
  slug: 'my-artifact',
  user_id: 'user-1',
  content: 'Some artifact content',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-02T00:00:00Z',
  status: 'active',
  title: 'My Artifact',
  description: 'desc',
  type: 'general',
  metadata: {},
}

function renderForm(overrides?: { artifact?: Artifact; onSubmit?: jest.Mock }) {
  const onSubmit = overrides?.onSubmit ?? jest.fn().mockResolvedValue(undefined)
  const ref = createRef<ArtifactFormHandle>()
  render(
    <ArtifactForm
      ref={ref}
      artifact={overrides?.artifact}
      onSubmit={onSubmit}
    />
  )
  return { onSubmit, ref }
}

describe('ArtifactForm — metadata editing', () => {
  it('renders the metadata editor', () => {
    renderForm()
    expect(screen.getByTestId('metadata-editor')).toBeInTheDocument()
  })

  it('pre-fills existing string metadata from the artifact', () => {
    renderForm({ artifact: { ...baseArtifact, metadata: { author: 'ada' } } })
    expect(screen.getByDisplayValue('author')).toBeInTheDocument()
    expect(screen.getByDisplayValue('ada')).toBeInTheDocument()
  })

  it('includes edited metadata in the update payload, preserving non-string values', async () => {
    const user = userEvent.setup()
    const { onSubmit, ref } = renderForm({
      artifact: { ...baseArtifact, metadata: { author: 'ada', tags: ['x'] } },
    })

    await user.click(screen.getByTestId('metadata-add-pair'))
    await user.type(screen.getByTestId('metadata-key-1'), 'env')
    await user.type(screen.getByTestId('metadata-value-1'), 'prod')

    await act(async () => {
      ref.current?.submit()
      await Promise.resolve()
    })

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledTimes(1)
    })
    expect(onSubmit.mock.calls[0][0]).toMatchObject({
      metadata: { author: 'ada', tags: ['x'], env: 'prod' },
    })
  })

  it('includes metadata in the create payload', async () => {
    const user = userEvent.setup()
    const { onSubmit, ref } = renderForm()

    await user.type(screen.getByTestId('artifact-title-input'), 'New Artifact')
    await user.type(
      screen.getByTestId('artifact-content-textarea'),
      'content body'
    )
    await user.click(screen.getByTestId('artifact-project-select'))

    await user.click(screen.getByTestId('metadata-add-pair'))
    await user.type(screen.getByTestId('metadata-key-0'), 'author')
    await user.type(screen.getByTestId('metadata-value-0'), 'grace')

    await act(async () => {
      ref.current?.submit()
      await Promise.resolve()
    })

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledTimes(1)
    })
    expect(onSubmit.mock.calls[0][0]).toMatchObject({
      slug: 'new-artifact',
      metadata: { author: 'grace' },
    })
  })

  it('omits metadata from the payload when empty', async () => {
    const { onSubmit, ref } = renderForm({
      artifact: { ...baseArtifact, metadata: {} },
    })
    await act(async () => {
      ref.current?.submit()
      await Promise.resolve()
    })
    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledTimes(1)
    })
    expect(onSubmit.mock.calls[0][0].metadata).toBeUndefined()
  })

  it('blocks submit when metadata is invalid', async () => {
    const user = userEvent.setup()
    const { onSubmit, ref } = renderForm({
      artifact: { ...baseArtifact, metadata: { author: 'ada' } },
    })

    await user.clear(screen.getByTestId('metadata-value-0'))
    await waitFor(() => {
      expect(screen.getByTestId('metadata-error-0')).toBeInTheDocument()
    })

    await act(async () => {
      ref.current?.submit()
      await Promise.resolve()
    })

    expect(onSubmit).not.toHaveBeenCalled()
  })
})
