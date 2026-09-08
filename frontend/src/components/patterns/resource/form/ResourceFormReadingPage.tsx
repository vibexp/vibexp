import { Info, Save, Tags, X } from 'lucide-react'
import type { ReactNode } from 'react'

import { useShell } from '@/components/layout/ShellContext'
import {
  type ReadingAction,
  ReadingPage,
  type ReadingSection,
} from '@/components/patterns/reading-page'
import { Form } from '@/components/ui/form'
import { useUnsavedChanges } from '@/hooks/useUnsavedChanges'

import { formSaveLabel } from './formLabels'
import type { ResourceFormPageProps } from './ResourceFormPage'
import { useResourceForm } from './useResourceForm'

/** Section ids, exported so tests and rail deep-links can address them. */
export const RESOURCE_FORM_SECTION_IDS = {
  details: 'form-details',
  taxonomy: 'form-taxonomy',
} as const

export interface ResourceFormReadingPageProps extends ResourceFormPageProps {
  /** The `<h1>` — "Edit artifact". */
  title: string
  /** Lead line under the title; the resource's own name, usually. */
  description?: ReactNode
  /** Where Cancel goes. Called only once the unsaved-changes guard clears. */
  onCancel: () => void
  /** Forwarded to the Save action, for pages an e2e spec addresses by id. */
  saveTestId?: string
}

/**
 * The generated form (#913) rendered in the reading shell (#916).
 *
 * View and edit are the same document, so they get the same layout: the body
 * editor takes the article slot at the identical 72ch measure and gutters, the
 * form's Details and Taxonomy controls become `ReadingSection`s in the details
 * column — folding to the icon rail from the same header toggle as on the
 * detail page — and Save/Cancel are `ReadingAction`s, so they render as the
 * column's button grid, as rail icons when it is folded, and as chips under the
 * title on phones.
 *
 * The form element lives in the article while the details controls render in
 * the `<aside>` beside it. That is fine and deliberate: `Form` is react-hook-
 * form's `FormProvider`, which renders no DOM of its own, and every control is
 * registered through that context rather than through DOM containment. Save
 * therefore submits the whole form no matter which slot a field sits in.
 */
export function ResourceFormReadingPage({
  title,
  description,
  onCancel,
  saveTestId,
  descriptor,
  mode,
  initialValues,
  onSubmit,
  isLoading = false,
  extensions,
  renderBody,
  metadataRequiredKeys,
  metadataReservedKeys,
}: Readonly<ResourceFormReadingPageProps>) {
  const { setDetailsOpen } = useShell()

  const {
    form,
    formElRef,
    onFormSubmit,
    submit,
    isDirty,
    bodyNode,
    detailsNode,
    taxonomyNode,
  } = useResourceForm({
    descriptor,
    mode,
    initialValues,
    onSubmit,
    isLoading,
    renderBody,
    metadataRequiredKeys,
    metadataReservedKeys,
    // Most fields live in the details column, and the column folds to a 48px
    // rail that renders none of them. A validation error the reader cannot
    // reach is one they cannot fix, so a failed submit reopens the column.
    onInvalidSubmit: () => {
      setDetailsOpen(true)
    },
  })

  const { confirmLeave } = useUnsavedChanges(isDirty && !isLoading)

  const extensionNodes = new Map(Object.entries(extensions ?? {}))
  const declaredExtensions = descriptor.form?.extensions ?? []

  const actions: ReadingAction[] = [
    {
      id: 'save',
      label: isLoading ? 'Saving…' : formSaveLabel(descriptor, mode),
      icon: Save,
      emphasis: 'primary',
      disabled: isLoading,
      onClick: submit,
      testId: saveTestId,
    },
    {
      id: 'cancel',
      label: 'Cancel',
      icon: X,
      disabled: isLoading,
      onClick: () => {
        if (confirmLeave()) onCancel()
      },
    },
  ]

  // The extension slots (the prompt's MCP exposure card, the memory's tags)
  // keep the position the standalone layout gives them — under the details
  // fields — rather than becoming rail entries of their own: they are
  // self-titled cards, and the descriptor declares only their names.
  const extensionCards = declaredExtensions
    .map(name => {
      const node = extensionNodes.get(name)
      return node ? <div key={name}>{node}</div> : null
    })
    .filter(Boolean)

  const sections: ReadingSection[] = [
    {
      id: RESOURCE_FORM_SECTION_IDS.details,
      label: 'Details',
      icon: Info,
      content: (detailsNode !== null || extensionCards.length > 0) && (
        <div className="space-y-4" data-testid="resource-form-details">
          {detailsNode}
          {extensionCards}
        </div>
      ),
    },
    {
      id: RESOURCE_FORM_SECTION_IDS.taxonomy,
      label: 'Labels & metadata',
      icon: Tags,
      content: taxonomyNode && (
        <div className="space-y-4" data-testid="resource-form-taxonomy">
          {taxonomyNode}
        </div>
      ),
    },
  ]

  return (
    <Form {...form}>
      <ReadingPage
        presentation="editing"
        title={title}
        description={description}
        actions={actions}
        sections={sections}
      >
        <form
          ref={formElRef}
          data-testid="resource-form"
          className="space-y-4"
          onSubmit={onFormSubmit}
        >
          {bodyNode}
        </form>
      </ReadingPage>
    </Form>
  )
}
