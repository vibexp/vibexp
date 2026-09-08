import { act, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createRef } from 'react'

// The picker is an async combobox over the projects API; stub it to a button
// that selects a fixed project, as the four page suites already do.
vi.mock('@/components/ProjectPicker', () => ({
  ProjectPicker: ({
    onChange,
    id,
    'data-testid': testId,
  }: {
    onChange: (id: string | null) => void
    id?: string
    'data-testid'?: string
  }) => (
    <button
      type="button"
      id={id}
      data-testid={testId}
      onClick={() => {
        onChange('p1')
      }}
    >
      select project
    </button>
  ),
}))

vi.mock('@/hooks/useTypes', () => ({
  useTypes: () => ({
    types: [{ id: 't1', slug: 'general', name: 'General' }],
    isLoading: false,
  }),
}))

import { artifactDescriptor } from '../../descriptors/artifact'
import { blueprintDescriptor } from '../../descriptors/blueprint'
import { memoryDescriptor } from '../../descriptors/memory'
import { promptDescriptor } from '../../descriptors/prompt'
import type { ResourceDescriptor } from '../../types'
import type {
  ResourceFormHandle,
  ResourceFormPageProps,
} from '../ResourceFormPage'
import { ResourceFormPage } from '../ResourceFormPage'

const FORM_KINDS: readonly (readonly [string, ResourceDescriptor])[] = [
  ['prompt', promptDescriptor],
  ['artifact', artifactDescriptor],
  ['blueprint', blueprintDescriptor],
  ['memory', memoryDescriptor],
]

function renderPage(
  descriptor: ResourceDescriptor,
  overrides: Partial<ResourceFormPageProps> = {}
) {
  const onSubmit = vi.fn().mockResolvedValue(undefined)
  const ref = createRef<ResourceFormHandle>()
  const view = render(
    <ResourceFormPage
      ref={ref}
      descriptor={descriptor}
      mode="create"
      onSubmit={onSubmit}
      {...overrides}
    />
  )
  return { onSubmit, ref, view }
}

async function submit(ref: React.RefObject<ResourceFormHandle | null>) {
  await act(async () => {
    ref.current?.submit()
    await new Promise(resolve => setTimeout(resolve, 0))
  })
}

describe('ResourceFormPage — every registered descriptor', () => {
  it.each(FORM_KINDS)('renders a create form for %s', (_kind, descriptor) => {
    renderPage(descriptor)
    expect(screen.getByTestId('resource-form')).toBeInTheDocument()
    for (const spec of descriptor.form?.fields ?? []) {
      if (spec.testId) {
        expect(screen.getByTestId(spec.testId)).toBeInTheDocument()
      }
    }
  })

  it.each(FORM_KINDS)('renders an edit form for %s', (_kind, descriptor) => {
    renderPage(descriptor, { mode: 'edit', initialValues: {} })
    expect(screen.getByTestId('resource-form')).toBeInTheDocument()
  })

  /**
   * The AC that this replaces four layouts with one: every kind puts its
   * details fields in the same card, in descriptor order, and grows a taxonomy
   * card exactly when it declares taxonomy fields — no per-kind sidebar.
   */
  it.each(FORM_KINDS)(
    'lays %s out in the shared sections',
    (_kind, descriptor) => {
      renderPage(descriptor)
      const specs = descriptor.form?.fields ?? []

      const details = screen.getByTestId('resource-form-details')
      expect(within(details).getByText('Details')).toBeInTheDocument()
      const detailLabels = [...details.querySelectorAll('label')].map(
        node => node.textContent
      )
      expect(detailLabels).toEqual(
        specs
          .filter(spec => spec.section === 'details')
          .map(spec => descriptor.fields.find(f => f.key === spec.key)?.label)
      )

      const taxonomySpecs = specs.filter(spec => spec.section === 'taxonomy')
      const taxonomy = screen.queryByTestId('resource-form-taxonomy')
      if (taxonomySpecs.length === 0) {
        expect(taxonomy).not.toBeInTheDocument()
        return
      }
      expect(taxonomy).toBeInTheDocument()
      expect(
        within(screen.getByTestId('resource-form-taxonomy')).getByText(
          'Labels & metadata'
        )
      ).toBeInTheDocument()
    }
  )
})

describe('ResourceFormPage — controls', () => {
  it('renders a text control for the name and a body control for the content', () => {
    renderPage(artifactDescriptor)
    expect(screen.getByTestId('artifact-title-input').tagName).toBe('INPUT')
    expect(screen.getByTestId('artifact-content-textarea').tagName).toBe(
      'TEXTAREA'
    )
  })

  it('renders a select trigger for a field-backed status', () => {
    renderPage(memoryDescriptor)
    expect(screen.getByTestId('memory-status-select')).toHaveTextContent(
      'Active'
    )
  })

  it('renders a select trigger for a runtime type catalog', () => {
    renderPage(artifactDescriptor, { initialValues: { type: 'general' } })
    expect(screen.getByTestId('artifact-type-select')).toHaveTextContent(
      'General'
    )
  })

  it('renders the shared project picker', () => {
    renderPage(blueprintDescriptor)
    expect(screen.getByTestId('blueprint-project-select')).toBeInTheDocument()
  })

  it('renders the shared metadata editor for a metadata control', () => {
    renderPage(memoryDescriptor)
    expect(screen.getByTestId('metadata-editor')).toBeInTheDocument()
  })

  it('renders a chip editor for a taxonomy control', async () => {
    const user = userEvent.setup()
    const { ref, onSubmit } = renderPage(promptDescriptor, {
      initialValues: {
        name: 'A prompt',
        slug: 'a-prompt',
        body: 'Body',
        project_id: 'p1',
      },
    })
    await user.type(screen.getByTestId('prompt-labels-input'), 'alpha{Enter}')
    expect(screen.getByText('alpha')).toBeInTheDocument()
    await submit(ref)
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ labels: ['alpha'] })
    )
  })

  it('replaces the body control when a body slot is supplied', () => {
    renderPage(memoryDescriptor, {
      renderBody: props => (
        <textarea data-testid="custom-body" value={props.value} readOnly />
      ),
    })
    expect(screen.getByTestId('custom-body')).toBeInTheDocument()
    expect(screen.queryByTestId('memory-text-textarea')).not.toBeInTheDocument()
  })
})

describe('ResourceFormPage — create versus edit', () => {
  it('auto-fills the slug from the name while creating', async () => {
    const user = userEvent.setup()
    renderPage(artifactDescriptor)
    await user.type(screen.getByTestId('artifact-title-input'), 'My Artifact')
    expect(screen.getByTestId('artifact-slug-input')).toHaveValue('my-artifact')
  })

  it('stops auto-filling once the slug is edited by hand', async () => {
    const user = userEvent.setup()
    renderPage(artifactDescriptor)
    await user.type(screen.getByTestId('artifact-slug-input'), 'chosen')
    await user.type(screen.getByTestId('artifact-title-input'), 'My Artifact')
    expect(screen.getByTestId('artifact-slug-input')).toHaveValue('chosen')
  })

  it('locks a create-only field while editing', () => {
    renderPage(artifactDescriptor, {
      mode: 'edit',
      initialValues: { slug: 'existing' },
    })
    expect(screen.getByTestId('artifact-slug-input')).toBeDisabled()
  })

  it('leaves a slug the kind keeps editable enabled while editing', () => {
    renderPage(promptDescriptor, {
      mode: 'edit',
      initialValues: { slug: 'existing' },
    })
    expect(screen.getByTestId('prompt-slug-input')).toBeEnabled()
  })

  it('never auto-fills the slug while editing', async () => {
    const user = userEvent.setup()
    renderPage(promptDescriptor, {
      mode: 'edit',
      initialValues: { name: 'Old', slug: 'kept-slug' },
    })
    await user.type(screen.getByTestId('prompt-name-input'), ' renamed')
    expect(screen.getByTestId('prompt-slug-input')).toHaveValue('kept-slug')
  })

  it('disables every control while the page is saving', () => {
    renderPage(memoryDescriptor, { isLoading: true })
    expect(screen.getByTestId('memory-text-textarea')).toBeDisabled()
  })
})

describe('ResourceFormPage — extensions', () => {
  it('renders a node for a slot the descriptor declares', () => {
    renderPage(promptDescriptor, {
      extensions: { 'mcp-exposure': <div data-testid="mcp-slot">MCP</div> },
    })
    expect(screen.getByTestId('mcp-slot')).toBeInTheDocument()
  })

  it('ignores a node for a slot the descriptor does not declare', () => {
    renderPage(promptDescriptor, {
      extensions: { unknown: <div data-testid="stray">stray</div> },
    })
    expect(screen.queryByTestId('stray')).not.toBeInTheDocument()
  })

  it('renders nothing for a declared slot the page did not fill', () => {
    renderPage(promptDescriptor)
    expect(screen.queryByTestId('mcp-slot')).not.toBeInTheDocument()
  })
})

describe('ResourceFormPage — submit', () => {
  it('hands the parsed values to onSubmit', async () => {
    const { ref, onSubmit } = renderPage(memoryDescriptor, {
      initialValues: { text: '  A memory  ', project_id: 'p1' },
    })
    await submit(ref)
    expect(onSubmit).toHaveBeenCalledTimes(1)
    expect(onSubmit).toHaveBeenCalledWith({
      // Trimmed by the schema, so a page never has to trim again.
      text: 'A memory',
      project_id: 'p1',
      status: 'active',
      metadata: {},
    })
  })

  it('blocks the submit while a required field is blank', async () => {
    const { ref, onSubmit } = renderPage(memoryDescriptor)
    await submit(ref)
    expect(onSubmit).not.toHaveBeenCalled()
    expect(await screen.findByText('Memory is required')).toBeInTheDocument()
  })

  it('blocks the submit while the metadata editor is invalid', async () => {
    const user = userEvent.setup()
    const { ref, onSubmit } = renderPage(memoryDescriptor, {
      initialValues: { text: 'A memory', project_id: 'p1' },
    })
    await user.click(screen.getByTestId('metadata-add-pair'))
    // A value with no key is the editor's canonical invalid row.
    await user.type(screen.getByTestId('metadata-value-0'), 'orphan')
    expect(await screen.findByTestId('metadata-error-0')).toBeInTheDocument()
    await submit(ref)
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('re-seeds when the resource resolves after first paint', () => {
    const { view } = renderPage(artifactDescriptor, {
      mode: 'edit',
      initialValues: {},
    })
    view.rerender(
      <ResourceFormPage
        descriptor={artifactDescriptor}
        mode="edit"
        initialValues={{ title: 'Loaded later' }}
        onSubmit={vi.fn()}
      />
    )
    expect(screen.getByTestId('artifact-title-input')).toHaveValue(
      'Loaded later'
    )
  })
})

/**
 * `FormControl` is a Radix `Slot` whose whole job is to clone `id`,
 * `aria-describedby` and `aria-invalid` onto the real input — a component in
 * between that does not forward them swallows them silently, leaving every
 * `FormLabel htmlFor` dangling and every validation message unannounced. The
 * forms this page replaces are queried by label in their own suites
 * (`BlueprintForm.test.tsx`), so losing it would be a regression that only
 * showed up as deleted assertions in #915.
 */
describe('ResourceFormPage — label and error association', () => {
  /**
   * Asserted through the LABEL's own `htmlFor`, not through `getByLabelText`:
   * the taxonomy and select controls already carried an `aria-label`, so a
   * label-text query passes on those two whether or not the slot's `id` ever
   * reaches the DOM — exactly the vacuous guard this fix needs to avoid.
   */
  it.each([
    ['artifact title (text)', artifactDescriptor, 'Title'],
    ['artifact description (textarea)', artifactDescriptor, 'Description'],
    ['artifact content (body)', artifactDescriptor, 'Content'],
    ['artifact status (select)', artifactDescriptor, 'Status'],
    ['artifact project (picker)', artifactDescriptor, 'Project'],
    ['prompt labels (taxonomy)', promptDescriptor, 'Labels'],
  ])('points the %s label at a real control', (_case, descriptor, label) => {
    renderPage(descriptor)
    const node = screen.getByText(label, { selector: 'label' })
    const target = node.getAttribute('for')
    expect(target).toBeTruthy()
    expect(document.getElementById(target ?? '')).not.toBeNull()
  })

  it('marks the control invalid and points the message at it', async () => {
    const { ref } = renderPage(artifactDescriptor)
    await submit(ref)
    const title = screen.getByLabelText('Title')
    expect(title).toHaveAttribute('aria-invalid', 'true')
    const message = await screen.findByText('Title is required')
    expect(title.getAttribute('aria-describedby')).toContain(message.id)
  })

  it('does not label the metadata editor, which has no single control', () => {
    renderPage(memoryDescriptor)
    // The heading is still rendered; it is simply not a <label for="…">.
    expect(screen.getByText('Metadata')).toBeInTheDocument()
    expect(screen.queryByLabelText('Metadata')).not.toBeInTheDocument()
  })
})

describe('ResourceFormPage — taxonomy bounds', () => {
  it('stops adding past the descriptor’s cap', async () => {
    const user = userEvent.setup()
    renderPage(promptDescriptor, {
      initialValues: {
        labels: Array.from({ length: 10 }, (_, i) => `label-${String(i)}`),
      },
    })
    expect(screen.getByText('10/10')).toBeInTheDocument()
    expect(screen.getByTestId('prompt-labels-input')).toBeDisabled()
    await user.click(screen.getByLabelText('Remove label-0'))
    expect(screen.getByTestId('prompt-labels-input')).toBeEnabled()
  })
})

describe('ResourceFormPage — re-seeding', () => {
  function editTree(initialValues: Record<string, unknown>) {
    return (
      <ResourceFormPage
        descriptor={artifactDescriptor}
        mode="edit"
        initialValues={initialValues}
        onSubmit={vi.fn()}
      />
    )
  }

  it('keeps typed input when the parent re-renders with an equal seed', async () => {
    const user = userEvent.setup()
    const view = render(editTree({ title: 'Loaded' }))
    await user.clear(screen.getByTestId('artifact-title-input'))
    await user.type(screen.getByTestId('artifact-title-input'), 'Typed by hand')
    // A fresh object with identical content — what every page produces on each
    // render. An identity check would reset the form and lose the edit.
    view.rerender(editTree({ title: 'Loaded' }))
    expect(screen.getByTestId('artifact-title-input')).toHaveValue(
      'Typed by hand'
    )
  })

  it('re-seeds when the seed’s content genuinely changes', () => {
    const view = render(editTree({ title: 'Loaded' }))
    view.rerender(editTree({ title: 'Reloaded' }))
    expect(screen.getByTestId('artifact-title-input')).toHaveValue('Reloaded')
  })
})

describe('ResourceFormPage — taxonomy entry length', () => {
  it('cannot create an entry longer than the descriptor allows', async () => {
    const user = userEvent.setup()
    const { ref, onSubmit } = renderPage(promptDescriptor, {
      initialValues: {
        name: 'A prompt',
        slug: 'a-prompt',
        body: 'Body',
        project_id: 'p1',
      },
    })
    const input = screen.getByTestId('prompt-labels-input')
    await user.type(input, `${'x'.repeat(60)}{Enter}`)
    // The schema caps entries too, but a zod error inside an array nests under
    // `labels[0]` and `FormMessage` renders nothing for it — so Save would
    // silently do nothing. The control has to make the entry impossible.
    expect(screen.getByText('x'.repeat(50))).toBeInTheDocument()
    await submit(ref)
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ labels: ['x'.repeat(50)] })
    )
  })
})
