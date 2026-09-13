import type { ResourceFormValues } from '../buildFormSchema'
import {
  enumValue,
  recordValue,
  stringListValue,
  stringValue,
} from '../formValues'

describe('recordValue', () => {
  it('reads an object value back out', () => {
    const values: ResourceFormValues = { metadata: { model: 'gpt-5' } }
    expect(recordValue(values, 'metadata')).toEqual({ model: 'gpt-5' })
  })

  it('is an empty object — not undefined — for a missing, null, or emptied bag', () => {
    expect(recordValue({}, 'metadata')).toEqual({})
    expect(recordValue({ metadata: null }, 'metadata')).toEqual({})
    expect(recordValue({ metadata: {} }, 'metadata')).toEqual({})
  })

  it('falls back to an empty object for a wrong-typed value', () => {
    expect(recordValue({ metadata: 'not an object' }, 'metadata')).toEqual({})
    expect(recordValue({ metadata: ['a', 'b'] }, 'metadata')).toEqual({})
  })
})

describe('stringValue', () => {
  it('reads a string value back out, and empty string otherwise', () => {
    expect(stringValue({ title: 'Hello' }, 'title')).toBe('Hello')
    expect(stringValue({}, 'title')).toBe('')
    expect(stringValue({ title: 7 }, 'title')).toBe('')
  })
})

describe('stringListValue', () => {
  it('reads a string array back out, and an empty array otherwise', () => {
    expect(stringListValue({ labels: ['a', 'b'] }, 'labels')).toEqual([
      'a',
      'b',
    ])
    expect(stringListValue({}, 'labels')).toEqual([])
    expect(stringListValue({ labels: 'not-an-array' }, 'labels')).toEqual([])
  })

  it('drops non-string entries rather than failing', () => {
    expect(stringListValue({ labels: ['a', 7, null] }, 'labels')).toEqual(['a'])
  })
})

describe('enumValue', () => {
  const ALLOWED = ['active', 'draft', 'archived'] as const

  it('narrows to the current value when it is one of the allowed values', () => {
    expect(enumValue({ status: 'draft' }, 'status', ALLOWED)).toBe('draft')
  })

  it('falls back to the first allowed value otherwise', () => {
    expect(enumValue({ status: 'bogus' }, 'status', ALLOWED)).toBe('active')
    expect(enumValue({}, 'status', ALLOWED)).toBe('active')
  })
})
