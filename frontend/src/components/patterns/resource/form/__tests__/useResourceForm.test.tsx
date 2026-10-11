/**
 * The generated form's behaviour — controls, create-vs-edit, extensions,
 * submit, re-seeding — driven through the one layout that renders it,
 * `ResourceFormReadingPage` (#1181 retired the standalone `ResourceFormPage`
 * these cases were written against). The layout itself — sections, rail,
 * actions, header — is `ResourceFormReadingPage.test.tsx`'s subject.
 */
import { act, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createRef } from 'react'

import { ShellProvider } from '@/components/layout/ShellContext'
import { mockViewportWidth } from '@/lib/testing/matchMedia'
import { TEST_LOCK } from '@/lib/testing/resourceLock'
import { storage } from '@/utils/storage'

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
import type { BodySlotProps } from '../ResourceFormControl'
import type {
  ResourceFormHandle,
  ResourceFormReadingPageProps,
} from '../ResourceFormReadingPage'
import { ResourceFormReadingPage } from '../ResourceFormReadingPage'

const FORM_KINDS: readonly (readonly [string, ResourceDescriptor])[] = [
  ['prompt', promptDescriptor],
  ['artifact', artifactDescriptor],
  ['blueprint', blueprintDescriptor],
  ['memory', memoryDescriptor],
]

type PageProps = Partial<ResourceFormReadingPageProps> &
  Pick<ResourceFormReadingPageProps, 'descriptor'>

function page(props: PageProps) {
  return (
    <ShellProvider>
      <ResourceFormReadingPage
        title={`Create ${props.descriptor.singular}`}
        mode="create"
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        {...props}
      />
    </ShellProvider>
  )
}

function renderPage(
  descriptor: ResourceDescriptor,
  overrides: Partial<ResourceFormReadingPageProps> = {}
) {
  const onSubmit = vi.fn().mockResolvedValue(undefined)
  const ref = createRef<ResourceFormHandle>()
  const view = render(page({ ref, descriptor, onSubmit, ...overrides }))
  return { onSubmit, ref, view }
}

let viewport: ReturnType<typeof mockViewportWidth>

beforeEach(() => {
  storage.clear()
  viewport = mockViewportWidth(1280)
})

afterEach(() => {
  viewport.restore()
})

async function submit(ref: React.RefObject<ResourceFormHandle | null>) {
  await act(async () => {
    ref.current?.submit()
    await new Promise(resolve => setTimeout(resolve, 0))
  })
}

describe('useResourceForm — every registered descriptor', () => {
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
})

describe('useResourceForm — controls', () => {
  it('renders the name inline in the header and a body control for the content', () => {
    renderPage(artifactDescriptor)
    // A textarea so a long name wraps on a phone (#1179).
    expect(screen.getByTestId('artifact-title-input').tagName).toBe('TEXTAREA')
    expect(screen.getByTestId('artifact-content-textarea').tagName).toBe(
      'TEXTAREA'
    )
  })

  it('renders a select trigger for the status, on its first system value', () => {
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
      renderBody: (props: BodySlotProps) => (
        <textarea data-testid="custom-body" value={props.value} readOnly />
      ),
    })
    expect(screen.getByTestId('custom-body')).toBeInTheDocument()
    expect(
      screen.queryByTestId('memory-content-textarea')
    ).not.toBeInTheDocument()
  })
})

describe('useResourceForm — create versus edit', () => {
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

  // A locked field is a read-only fact while editing (#1180): the reading
  // page's copyable row, not a disabled input.
  it('locks a create-only field while editing', () => {
    renderPage(artifactDescriptor, {
      mode: 'edit',
      initialValues: { slug: 'existing' },
    })
    expect(screen.queryByTestId('artifact-slug-input')).not.toBeInTheDocument()
    // Scoped: the header's badge row carries its own copy chip.
    expect(
      within(screen.getByTestId('metadata-panel')).getByRole('button', {
        name: /copy slug/i,
      })
    ).toHaveTextContent('existing')
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
    expect(screen.getByTestId('memory-content-textarea')).toBeDisabled()
  })
})

describe('useResourceForm — extensions', () => {
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

describe('useResourceForm — submit', () => {
  it('hands the parsed values to onSubmit', async () => {
    const { ref, onSubmit } = renderPage(memoryDescriptor, {
      initialValues: { text: '  A memory  ', project_id: 'p1' },
    })
    await submit(ref)
    expect(onSubmit).toHaveBeenCalledTimes(1)
    expect(onSubmit).toHaveBeenCalledWith({
      // Trimmed by the schema, so a page never has to trim again.
      text: 'A memory',
      // Every declared field is present whether the resource carried it or
      // not, so a control never starts uncontrolled — an optional text field
      // the resource has no value for parses to the empty string, and it is
      // the page's job to decide whether that means "omit" or "clear".
      title: '',
      project_id: 'p1',
      status: 'active',
      labels: [],
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
      page({
        descriptor: artifactDescriptor,
        mode: 'edit',
        initialValues: { title: 'Loaded later' },
      })
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
describe('useResourceForm — label and error association', () => {
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
    expect(
      within(screen.getByTestId('resource-form-taxonomy')).getByText('Metadata')
    ).toBeInTheDocument()
    // Scoped: the column's Metadata SECTION is aria-labelled by design.
    expect(
      within(screen.getByTestId('resource-form-taxonomy')).queryByLabelText(
        'Metadata'
      )
    ).not.toBeInTheDocument()
  })
})

describe('useResourceForm — taxonomy bounds', () => {
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

describe('useResourceForm — re-seeding', () => {
  function editTree(initialValues: Record<string, unknown>) {
    return page({ descriptor: artifactDescriptor, mode: 'edit', initialValues })
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

describe('useResourceForm — taxonomy entry length', () => {
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

/**
 * What a lock freezes on each kind's edit page (#1285), by the test ids of the
 * controls involved. `rows` are details-column fields, which become read-only
 * rows like a create-only slug; `disabled` are the Labels & metadata controls,
 * which stay in place but disabled; `editable` is the content a lock never
 * touches. A blueprint's and an artifact's slug is already fixed after create.
 */
const LOCK_CASES = [
  {
    kind: 'prompt',
    descriptor: promptDescriptor,
    rows: [
      'prompt-slug-input',
      'prompt-status-select',
      'prompt-project-select',
    ],
    disabled: ['prompt-labels-input'],
    editable: [
      'prompt-name-input',
      'prompt-description-input',
      'prompt-body-textarea',
    ],
    notice:
      'Slug, Status, Labels, MCP, and Project cannot be changed until this prompt is unlocked.',
  },
  {
    kind: 'blueprint',
    descriptor: blueprintDescriptor,
    rows: [
      'blueprint-status-select',
      'blueprint-type-select',
      'blueprint-project-select',
    ],
    disabled: ['blueprint-labels-input', 'metadata-add-pair'],
    editable: [
      'blueprint-title-input',
      'blueprint-description-input',
      'blueprint-content-textarea',
    ],
    notice:
      'Project, Slug, Status, Type, Subtype, Labels, Path, and Metadata cannot be changed until this blueprint is unlocked.',
  },
  {
    kind: 'artifact',
    descriptor: artifactDescriptor,
    rows: [
      'artifact-status-select',
      'artifact-type-select',
      'artifact-project-select',
    ],
    disabled: ['artifact-labels-input', 'metadata-add-pair'],
    editable: [
      'artifact-title-input',
      'artifact-description-input',
      'artifact-content-textarea',
    ],
    notice:
      'Project, Slug, Status, Type, Labels, and Metadata cannot be changed until this artifact is unlocked.',
  },
] as const

describe.each(LOCK_CASES)(
  'useResourceForm — a locked $kind',
  ({ descriptor, rows, disabled, editable, notice }) => {
    const stored = {
      slug: 'stored-slug',
      project_id: 'p0',
      status: 'active',
      type: 'general',
    }

    it('freezes the fields the lock covers and leaves the content editable', () => {
      renderPage(descriptor, {
        mode: 'edit',
        initialValues: stored,
        lock: TEST_LOCK,
      })
      for (const testId of rows) {
        expect(screen.queryByTestId(testId)).not.toBeInTheDocument()
      }
      for (const testId of disabled) {
        expect(screen.getByTestId(testId)).toBeDisabled()
      }
      for (const testId of editable) {
        expect(screen.getByTestId(testId)).toBeEnabled()
      }
    })

    it('says who locked it, why, and which fields that freezes', () => {
      renderPage(descriptor, {
        mode: 'edit',
        initialValues: stored,
        lock: TEST_LOCK,
      })
      const lockNotice = screen.getByTestId('resource-lock-notice')
      expect(lockNotice).toHaveTextContent(
        'Locked by Ada Lovelace: Used by the nightly release agent'
      )
      expect(lockNotice).toHaveTextContent(notice)
    })

    it('renders the edit page as before when there is no lock', () => {
      renderPage(descriptor, {
        mode: 'edit',
        initialValues: stored,
        lock: null,
      })
      expect(
        screen.queryByTestId('resource-lock-notice')
      ).not.toBeInTheDocument()
      for (const testId of [...disabled, ...editable]) {
        expect(screen.getByTestId(testId)).toBeEnabled()
      }
      // A slug fixed after create is a row with or without a lock.
      const createOnly = descriptor.form.fields
        .filter(spec => 'editableOnCreateOnly' in spec)
        .map(spec => spec.testId)
      for (const testId of rows.filter(id => !createOnly.includes(id))) {
        expect(screen.getByTestId(testId)).toBeEnabled()
      }
    })

    it('still submits the frozen fields, with their stored values', async () => {
      const { ref, onSubmit } = renderPage(descriptor, {
        mode: 'edit',
        initialValues: {
          ...stored,
          name: 'A name',
          title: 'A title',
          body: 'Body',
          content: 'Content',
          labels: ['kept'],
        },
        lock: TEST_LOCK,
      })
      await submit(ref)
      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          slug: 'stored-slug',
          project_id: 'p0',
          status: 'active',
          labels: ['kept'],
        })
      )
    })
  }
)

describe('useResourceForm — a lock and the create page', () => {
  it('ignores a lock while creating: there is no stored resource to freeze', () => {
    renderPage(artifactDescriptor, { lock: TEST_LOCK })
    expect(screen.queryByTestId('resource-lock-notice')).not.toBeInTheDocument()
    expect(screen.getByTestId('artifact-status-select')).toBeEnabled()
    expect(screen.getByTestId('artifact-labels-input')).toBeEnabled()
  })
})

// The lock race: the page loaded unlocked, the save was refused with a 423,
// and the page refetched the resource to learn about the lock.
describe('useResourceForm — a lock that arrives after the page loaded', () => {
  const loaded = {
    title: 'Loaded',
    slug: 'an-artifact',
    content: 'Content',
    project_id: 'p0',
    type: 'general',
    status: 'active',
  }

  function editTree(props: Partial<ResourceFormReadingPageProps> = {}) {
    const ref = createRef<ResourceFormHandle>()
    const onSubmit = vi.fn().mockResolvedValue(undefined)
    const onCancel = vi.fn()
    const tree = (extra: Partial<ResourceFormReadingPageProps> = {}) =>
      page({
        ref,
        onSubmit,
        onCancel,
        descriptor: artifactDescriptor,
        mode: 'edit',
        initialValues: loaded,
        ...props,
        ...extra,
      })
    return { ref, onSubmit, onCancel, tree }
  }

  it('takes back a change to a field the lock froze and keeps the content edits', async () => {
    const user = userEvent.setup()
    const { ref, onSubmit, tree } = editTree()
    const view = render(tree())
    await user.type(screen.getByTestId('artifact-title-input'), ' and typed')
    // The stubbed picker moves the artifact to project `p1`.
    await user.click(screen.getByTestId('artifact-project-select'))

    view.rerender(tree({ lock: TEST_LOCK, record: loaded }))

    expect(screen.getByTestId('artifact-title-input')).toHaveValue(
      'Loaded and typed'
    )
    expect(
      screen.queryByTestId('artifact-project-select')
    ).not.toBeInTheDocument()
    await submit(ref)
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ title: 'Loaded and typed', project_id: 'p0' })
    )
  })

  it('submits what the server holds when the frozen fields changed under the page', async () => {
    const { ref, onSubmit, tree } = editTree()
    const view = render(tree())

    view.rerender(
      tree({
        lock: TEST_LOCK,
        record: { ...loaded, project_id: 'p9', status: 'archived' },
      })
    )

    await submit(ref)
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ project_id: 'p9', status: 'archived' })
    )
  })

  it('takes the change back even when its control is not mounted', async () => {
    // Below `lg` the details column is a closed sheet, so the project control
    // the reader changed is unmounted by the time the lock arrives.
    const user = userEvent.setup()
    const { ref, onSubmit, tree } = editTree()
    const view = render(tree())
    await user.click(screen.getByTestId('artifact-project-select'))
    act(() => {
      viewport.restore()
      viewport = mockViewportWidth(600)
    })
    view.rerender(tree())
    expect(
      screen.queryByTestId('artifact-project-select')
    ).not.toBeInTheDocument()

    view.rerender(tree({ lock: TEST_LOCK, record: loaded }))

    await submit(ref)
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ project_id: 'p0' })
    )
  })

  it('no longer counts the taken-back change as unsaved', async () => {
    const user = userEvent.setup()
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const { onCancel, tree } = editTree()
    const view = render(tree())
    await user.click(screen.getByTestId('artifact-project-select'))

    view.rerender(tree({ lock: TEST_LOCK, record: loaded }))
    await user.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(confirmSpy).not.toHaveBeenCalled()
    expect(onCancel).toHaveBeenCalledTimes(1)
    confirmSpy.mockRestore()
  })
})
