import { artifactDescriptor } from '../../descriptors/artifact'
import { blueprintDescriptor } from '../../descriptors/blueprint'
import { galleryPromptDescriptor } from '../../descriptors/galleryPrompt'
import { memoryDescriptor } from '../../descriptors/memory'
import { promptDescriptor } from '../../descriptors/prompt'
import { resourceRegistry } from '../../registry'
import type { ResourceDescriptor } from '../../types'
import {
  buildFormSchema,
  defaultFormValues,
  entryMaxLengthMessage,
  maxItemsMessage,
  maxLengthMessage,
  requiredMessage,
  SLUG_MESSAGE,
  slugify,
} from '../buildFormSchema'

/** The first error message zod reports for a path, or undefined when valid. */
function errorAt(
  descriptor: ResourceDescriptor,
  values: Record<string, unknown>,
  path: string
): string | undefined {
  const result = buildFormSchema(descriptor).safeParse(values)
  if (result.success) return undefined
  return result.error.issues.find(issue => issue.path.at(0) === path)?.message
}

function valid(descriptor: ResourceDescriptor): Record<string, unknown> {
  return {
    ...defaultFormValues(descriptor),
    ...(descriptor.kind === 'memory'
      ? { text: 'A memory', project_id: 'p1' }
      : {}),
    ...(descriptor.kind === 'prompt'
      ? {
          name: 'A prompt',
          slug: 'a-prompt',
          body: 'Body',
          project_id: 'p1',
        }
      : {}),
    ...(descriptor.kind === 'artifact' || descriptor.kind === 'blueprint'
      ? {
          title: 'A thing',
          slug: 'a-thing',
          content: 'Content',
          project_id: 'p1',
          type: 'general',
        }
      : {}),
  }
}

describe('buildFormSchema', () => {
  describe('required fields', () => {
    it.each([
      ['prompt', promptDescriptor, 'name', 'Name'],
      ['artifact', artifactDescriptor, 'title', 'Title'],
      ['blueprint', blueprintDescriptor, 'title', 'Title'],
      ['memory', memoryDescriptor, 'text', 'Memory'],
    ])(
      'uses one phrasing for a blank %s identifier',
      (_kind, descriptor, key, label) => {
        expect(
          errorAt(descriptor, { ...valid(descriptor), [key]: '' }, key)
        ).toBe(requiredMessage(label))
      }
    )

    it('rejects a whitespace-only value, because strings are trimmed', () => {
      expect(
        errorAt(
          memoryDescriptor,
          { ...valid(memoryDescriptor), text: '   ' },
          'text'
        )
      ).toBe(requiredMessage('Memory'))
    })

    it('accepts a fully populated resource', () => {
      for (const descriptor of Object.values(resourceRegistry)) {
        if (!descriptor.form) continue
        expect(
          buildFormSchema(descriptor).safeParse(valid(descriptor)).success
        ).toBe(true)
      }
    })

    it('leaves an optional field absent rather than requiring it', () => {
      const values = { ...valid(artifactDescriptor) }
      delete values.description
      expect(
        buildFormSchema(artifactDescriptor).safeParse(values).success
      ).toBe(true)
    })
  })

  describe('max length', () => {
    it('caps the summary at the descriptor’s own limit', () => {
      expect(
        errorAt(
          artifactDescriptor,
          { ...valid(artifactDescriptor), description: 'x'.repeat(501) },
          'description'
        )
      ).toBe(maxLengthMessage('Description', 500))
    })

    it('uses the same message with the kind’s own shorter limit', () => {
      expect(
        errorAt(
          promptDescriptor,
          { ...valid(promptDescriptor), description: 'x'.repeat(201) },
          'description'
        )
      ).toBe(maxLengthMessage('Description', 200))
    })

    it('uses the prompt’s own 50-character name cap, not the 255 of a title', () => {
      expect(
        errorAt(
          promptDescriptor,
          { ...valid(promptDescriptor), name: 'x'.repeat(51) },
          'name'
        )
      ).toBe(maxLengthMessage('Name', 50))
      expect(
        errorAt(
          artifactDescriptor,
          { ...valid(artifactDescriptor), title: 'x'.repeat(51) },
          'title'
        )
      ).toBeUndefined()
    })

    it('accepts a value exactly at the limit', () => {
      expect(
        errorAt(
          artifactDescriptor,
          { ...valid(artifactDescriptor), description: 'x'.repeat(500) },
          'description'
        )
      ).toBeUndefined()
    })
  })

  describe('slug pattern', () => {
    it.each([
      ['artifact', artifactDescriptor],
      ['blueprint', blueprintDescriptor],
      ['prompt', promptDescriptor],
    ])('reports one message on every kind (%s)', (_kind, descriptor) => {
      expect(
        errorAt(
          descriptor,
          { ...valid(descriptor), slug: 'Not A Slug' },
          'slug'
        )
      ).toBe(SLUG_MESSAGE)
    })

    it('accepts lowercase letters, numbers and dashes', () => {
      expect(
        errorAt(
          artifactDescriptor,
          { ...valid(artifactDescriptor), slug: 'a-slug-9' },
          'slug'
        )
      ).toBeUndefined()
    })
  })

  describe('select fields', () => {
    it('rejects a status value the resource cannot be in', () => {
      expect(
        errorAt(
          memoryDescriptor,
          { ...valid(memoryDescriptor), status: 'published' },
          'status'
        )
      ).toBeDefined()
    })

    it('accepts every value the descriptor enumerates', () => {
      for (const status of ['active', 'draft', 'archived']) {
        expect(
          errorAt(
            memoryDescriptor,
            { ...valid(memoryDescriptor), status },
            'status'
          )
        ).toBeUndefined()
      }
    })

    it('accepts any non-empty value for a runtime type catalog', () => {
      expect(
        errorAt(
          artifactDescriptor,
          { ...valid(artifactDescriptor), type: 'a-team-type' },
          'type'
        )
      ).toBeUndefined()
    })

    it('still requires a value from the runtime catalog', () => {
      expect(
        errorAt(
          artifactDescriptor,
          { ...valid(artifactDescriptor), type: '' },
          'type'
        )
      ).toBe(requiredMessage('Type'))
    })
  })

  describe('list and map fields', () => {
    it('takes a list of strings for a taxonomy control', () => {
      expect(
        buildFormSchema(promptDescriptor).safeParse({
          ...valid(promptDescriptor),
          labels: ['one', 'two'],
        }).success
      ).toBe(true)
    })

    it('caps the number of taxonomy entries at the descriptor’s limit', () => {
      expect(
        errorAt(
          promptDescriptor,
          {
            ...valid(promptDescriptor),
            labels: Array.from({ length: 11 }, (_, i) => `l${String(i)}`),
          },
          'labels'
        )
      ).toBe(maxItemsMessage('Labels', 10))
    })

    it('caps the length of each taxonomy entry', () => {
      expect(
        errorAt(
          promptDescriptor,
          { ...valid(promptDescriptor), labels: ['x'.repeat(51)] },
          'labels'
        )
      ).toBe(entryMaxLengthMessage('Labels', 50))
    })

    it('accepts a full but legal taxonomy list', () => {
      expect(
        errorAt(
          promptDescriptor,
          {
            ...valid(promptDescriptor),
            labels: Array.from({ length: 10 }, (_, i) => `l${String(i)}`),
          },
          'labels'
        )
      ).toBeUndefined()
    })

    it('rejects a taxonomy value that is not a list of strings', () => {
      expect(
        buildFormSchema(promptDescriptor).safeParse({
          ...valid(promptDescriptor),
          labels: 'one',
        }).success
      ).toBe(false)
    })

    it('takes the free-form map for a metadata control', () => {
      expect(
        buildFormSchema(artifactDescriptor).safeParse({
          ...valid(artifactDescriptor),
          metadata: { owner: 'ada', count: 2 },
        }).success
      ).toBe(true)
    })
  })

  describe('descriptors with no form', () => {
    it('produces an empty schema for a read-only kind', () => {
      const gallery: ResourceDescriptor = galleryPromptDescriptor
      expect(gallery.form).toBeUndefined()
      expect(buildFormSchema(gallery).safeParse({}).success).toBe(true)
    })
  })
})

describe('defaultFormValues', () => {
  it('gives every declared field an entry, so no control starts uncontrolled', () => {
    const values = defaultFormValues(artifactDescriptor)
    expect(Object.keys(values).sort()).toEqual(
      [...artifactDescriptor.form.fields].map(field => field.key).sort()
    )
  })

  it('opens a field-backed select on the descriptor’s first value', () => {
    expect(defaultFormValues(memoryDescriptor).status).toBe('active')
    expect(defaultFormValues(promptDescriptor).status).toBe('draft')
    expect(defaultFormValues(blueprintDescriptor).type).toBe('general')
  })

  it('leaves a runtime-catalog select empty — the catalog is not known yet', () => {
    expect(defaultFormValues(artifactDescriptor).type).toBe('')
  })

  it('seeds from an existing resource', () => {
    const values = defaultFormValues(artifactDescriptor, {
      title: 'Existing',
      slug: 'existing',
      metadata: { owner: 'ada' },
    })
    expect(values.title).toBe('Existing')
    expect(values.metadata).toEqual({ owner: 'ada' })
  })

  it('coerces a wrong-shaped value to the control’s empty value', () => {
    const values = defaultFormValues(promptDescriptor, {
      name: 42,
      labels: ['keep', 7],
    })
    expect(values.name).toBe('')
    expect(values.labels).toEqual(['keep'])
  })

  it('replaces a null metadata blob with an empty map', () => {
    expect(
      defaultFormValues(memoryDescriptor, { metadata: null }).metadata
    ).toEqual({})
  })
})

describe('slugify', () => {
  it.each([
    ['My Artifact', 'my-artifact'],
    ['  Leading and trailing  ', 'leading-and-trailing'],
    ['Punctuation!! everywhere??', 'punctuation-everywhere'],
    ['Already-a-slug', 'already-a-slug'],
    ['', ''],
  ])('slugifies %j to %j', (input, expected) => {
    expect(slugify(input)).toBe(expected)
  })
})
