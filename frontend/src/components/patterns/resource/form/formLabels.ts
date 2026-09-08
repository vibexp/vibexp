import { fieldOfRole } from '../fieldOfRole'
import type { ResourceDescriptor } from '../types'
import type { ResourceFormValues } from './buildFormSchema'

/** Whether a form is creating a resource or editing an existing one. */
export type ResourceFormMode = 'create' | 'edit'

/*
 * The wording around a generated form, derived from the descriptor.
 *
 * The page header is not part of `ResourceFormPage` — every create/edit page
 * owns its own `PageHeader` (with Back, Save, and the page's navigation), and
 * the `forwardRef` submit handle exists precisely so the Save button can live
 * up there. These helpers are what stop the header drifting from the form
 * below it: today the four pages read "Create artifact", "Create Blueprint",
 * "Save Prompt" and "Update memory".
 */

/** "Create artifact" / "Edit artifact". */
export function formHeading(
  descriptor: ResourceDescriptor,
  mode: ResourceFormMode
): string {
  const verb = mode === 'create' ? 'Create' : 'Edit'
  return `${verb} ${descriptor.singular}`
}

/** "Create artifact" while creating, "Save changes" while editing. */
export function formSaveLabel(
  descriptor: ResourceDescriptor,
  mode: ResourceFormMode
): string {
  return mode === 'create' ? formHeading(descriptor, 'create') : 'Save changes'
}

/**
 * The resource's own name, for the header subtitle — so an edit page says
 * which resource is being edited. `undefined` when creating, when the kind has
 * no name field, or when the name is still blank.
 *
 * Returned raw: a memory's "name" is its whole text, and excerpting markdown is
 * `lib/markdownExcerpt`'s job, not this module's.
 */
export function formSubtitle(
  descriptor: ResourceDescriptor,
  values: ResourceFormValues | undefined
): string | undefined {
  const field = fieldOfRole(descriptor, 'name')
  if (!field || !values) return undefined
  const value = new Map(Object.entries(values)).get(field.key)
  if (typeof value !== 'string' || value.trim() === '') return undefined
  return value
}
