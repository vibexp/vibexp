import type { ReactNode } from 'react'
import { forwardRef, useImperativeHandle } from 'react'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Form } from '@/components/ui/form'

import type { ResourceDescriptor } from '../types'
import type { ResourceFormValues } from './buildFormSchema'
import type { ResourceFormMode } from './formLabels'
import type { BodySlotProps } from './ResourceFormControl'
import { useResourceForm } from './useResourceForm'

/** The handle the page header's Save button drives. */
export interface ResourceFormHandle {
  submit: () => void
  /**
   * The form's current values.
   *
   * The page owns the extension slots but not the form, and an extension
   * occasionally has to read it: the prompt editor's template loader must not
   * replace a body the user has already typed into, and re-seeding through
   * `initialValues` would otherwise discard every OTHER field along with it.
   * Reading, never writing — a slot that wants to change a field re-seeds.
   */
  getValues: () => ResourceFormValues
}

export interface ResourceFormPageProps {
  descriptor: ResourceDescriptor
  mode: ResourceFormMode
  /** The resource being edited, as a value map. Omit when creating. */
  initialValues?: ResourceFormValues
  onSubmit: (values: ResourceFormValues) => void | Promise<void>
  /** Disables every control while the page is saving. */
  isLoading?: boolean
  /**
   * A node per name in `descriptor.form.extensions`, rendered under the
   * details card in the order the descriptor declares. A name with no node is
   * simply not rendered — a page may fill one slot and not another.
   */
  extensions?: Readonly<Record<string, ReactNode>>
  /**
   * Replaces the shared `ResourceBodyEditor` the body control renders by
   * default — how a page opts into the prompt-only extensions (#914).
   */
  renderBody?: (props: BodySlotProps) => ReactNode
  /** Forwarded to the `metadata` control. Must be a stable reference. */
  metadataRequiredKeys?: string[]
  /** Forwarded to the `metadata` control. Must be a stable reference. */
  metadataReservedKeys?: string[]
}

/**
 * The one create/edit page, generated from `descriptor.form` (#913).
 *
 * `ArtifactForm` (357 lines), `MemoryForm` (341), `BlueprintForm` (302) and
 * `PromptEditor` (361) each hand-wrote a zod schema, a slugifier and a card
 * layout for the same six or seven fields, in three different architectures —
 * so "add a field to a resource" was a four-file change and the four pages had
 * already diverged on validation, on which sidebar cards exist and on whether
 * the slug auto-fills at all. Which fields a resource edits is a property of
 * the resource type now, and this page is the only reader.
 *
 * The form itself lives in `useResourceForm`, which hands back the body,
 * details and taxonomy controls as nodes; this component is only the standalone
 * card grid around them. `ResourceFormReadingPage` renders the same nodes into
 * the reading shell (#916) — one form, two layouts.
 *
 * ## What this does NOT own
 *
 * The page header. Every create route keeps its own `PageHeader` with the Back
 * and Save buttons, which is why the form exposes a `{ submit }` handle through
 * `forwardRef` — the pattern all three react-hook-form pages already use.
 * `formSaveLabel` / `formHeading` / `formSubtitle` derive that header's wording
 * from the same descriptor. (The four EDIT routes moved to the reading shell in
 * #916 and take their Save/Cancel from the actions rail instead.)
 *
 * Fetching, navigation and the request payload stay with the page too: this
 * hands `onSubmit` the parsed values, keyed by field key.
 */
export const ResourceFormPage = forwardRef<
  ResourceFormHandle,
  ResourceFormPageProps
>(function ResourceFormPage(
  {
    descriptor,
    mode,
    initialValues,
    onSubmit,
    isLoading = false,
    extensions,
    renderBody,
    metadataRequiredKeys,
    metadataReservedKeys,
  },
  ref
) {
  const {
    form,
    formElRef,
    onFormSubmit,
    submit,
    getValues,
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
  })

  useImperativeHandle(ref, () => ({ submit, getValues }), [submit, getValues])

  const extensionNodes = new Map(Object.entries(extensions ?? {}))

  return (
    <Form {...form}>
      <form
        ref={formElRef}
        data-testid="resource-form"
        className="grid gap-6 lg:grid-cols-3"
        onSubmit={onFormSubmit}
      >
        <div className="min-w-0 space-y-4 lg:col-span-2">{bodyNode}</div>
        <div className="min-w-0 space-y-4">
          {detailsNode && (
            <Card data-testid="resource-form-details">
              <CardHeader>
                <CardTitle className="text-sm">Details</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">{detailsNode}</CardContent>
            </Card>
          )}
          {(descriptor.form?.extensions ?? []).map(name => {
            const node = extensionNodes.get(name)
            return node ? <div key={name}>{node}</div> : null
          })}
          {taxonomyNode && (
            <Card data-testid="resource-form-taxonomy">
              <CardHeader>
                <CardTitle className="text-sm">Labels &amp; metadata</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">{taxonomyNode}</CardContent>
            </Card>
          )}
        </div>
      </form>
    </Form>
  )
})
