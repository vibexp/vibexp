import type { FormFieldSpec } from './types'

/**
 * Builders for the form fields every resource create/edit page shares.
 *
 * The same lesson `filterSpecs.ts` records: four descriptors restating the
 * same title / slug / summary / project / status entries by hand is still
 * duplication, it drifts exactly the way four components did, and Sonar's
 * new-code duplication gate counts declarative copies (#908).
 *
 * Resource-specific entries — the artifact type catalog, the prompt taxonomy —
 * stay written out on their descriptor: they are the part that differs, and a
 * builder used once is just indirection.
 */

/** Every kind's address slug is capped at 255 by the API. */
const SLUG_MAX = 255

/**
 * The human-readable identifier: a prompt's `name`, everyone else's `title`.
 *
 * `maxLength` is a parameter and not a shared constant because the limit is
 * NOT uniform — a prompt name is capped at 50 and every title at 255
 * (`backend/schemas/*.yaml`, the `Create*Request` schemas). Assuming otherwise
 * is exactly the drift `resourceFormSpec.test.ts` now pins against the spec.
 */
export function nameFormField(
  key: string,
  maxLength: number,
  testId: string
): FormFieldSpec {
  return {
    key,
    control: 'text',
    section: 'details',
    required: true,
    maxLength,
    testId,
  }
}

/**
 * The URL segment. `editableOnCreateOnly` is a per-kind fact rather than a
 * default: an artifact's slug is half its address and is locked after create,
 * a prompt's stays editable.
 */
export function slugFormField(
  testId: string,
  editableOnCreateOnly: boolean
): FormFieldSpec {
  return {
    key: 'slug',
    control: 'text',
    section: 'details',
    required: true,
    maxLength: SLUG_MAX,
    pattern: 'slug',
    placeholder: 'my-resource',
    description: editableOnCreateOnly
      ? 'Identifier used in URLs. Cannot be changed after creation.'
      : 'Identifier used in URLs.',
    editableOnCreateOnly,
    testId,
  }
}

/** The one-line description under the name. Optional on every kind. */
export function summaryFormField(
  key: string,
  maxLength: number,
  testId: string
): FormFieldSpec {
  return {
    key,
    control: 'textarea',
    section: 'details',
    maxLength,
    placeholder: 'Enter a brief description…',
    testId,
  }
}

/** The long-form content, in the wide editor column. */
export function bodyFormField(key: string, testId: string): FormFieldSpec {
  return { key, control: 'body', section: 'body', required: true, testId }
}

/** The owning project, through the shared picker on every kind. */
export function projectFormField(testId: string): FormFieldSpec {
  return {
    key: 'project_id',
    control: 'project',
    section: 'details',
    required: true,
    placeholder: 'Select project',
    testId,
  }
}

/**
 * The lifecycle status, read off the kind's own `status` field.
 *
 * `description` is a parameter because what a status MEANS is per-kind and the
 * copy is not always worth having: an artifact and a memory both hide drafts
 * from search, a blueprint expires, a prompt publishes. Only the kinds that had
 * the sentence keep it — it was the one place the UI said what the values do.
 */
export function statusFormField(
  kind: string,
  description?: string
): FormFieldSpec {
  return {
    key: 'status',
    control: 'select',
    section: 'details',
    required: true,
    optionsFrom: 'field',
    description,
    testId: `${kind}-status-select`,
  }
}

/**
 * A label list, through the shared chip editor. Both bounds are the API's
 * (`maxItems: 10`, `items.maxLength: 50` on every labelled resource).
 */
export function labelsFormField(key: string, testId: string): FormFieldSpec {
  return {
    key,
    control: 'taxonomy',
    section: 'taxonomy',
    maxItems: 10,
    maxLength: 50,
    placeholder: 'Add a label…',
    testId,
  }
}

/** The free-form key/value bag, through the shared `MetadataEditor`. */
export function metadataFormField(): FormFieldSpec {
  return { key: 'metadata', control: 'metadata', section: 'taxonomy' }
}
