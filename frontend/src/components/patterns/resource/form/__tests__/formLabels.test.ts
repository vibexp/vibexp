import { artifactDescriptor } from '../../descriptors/artifact'
import { memoryDescriptor } from '../../descriptors/memory'
import { promptDescriptor } from '../../descriptors/prompt'
import { formHeading, formSaveLabel, formSubtitle } from '../formLabels'

describe('formHeading', () => {
  it('names the kind from the descriptor', () => {
    expect(formHeading(artifactDescriptor, 'create')).toBe('Create artifact')
    expect(formHeading(memoryDescriptor, 'edit')).toBe('Edit memory')
  })
})

describe('formSaveLabel', () => {
  it('reads "Create <singular>" while creating', () => {
    expect(formSaveLabel(promptDescriptor, 'create')).toBe('Create prompt')
    expect(formSaveLabel(artifactDescriptor, 'create')).toBe('Create artifact')
  })

  it('reads "Save changes" while editing, on every kind', () => {
    for (const descriptor of [
      promptDescriptor,
      artifactDescriptor,
      memoryDescriptor,
    ]) {
      expect(formSaveLabel(descriptor, 'edit')).toBe('Save changes')
    }
  })
})

describe('formSubtitle', () => {
  it('is the resource’s own name', () => {
    expect(formSubtitle(artifactDescriptor, { title: 'My artifact' })).toBe(
      'My artifact'
    )
  })

  it('reads the name field of a kind that calls it something else', () => {
    expect(formSubtitle(promptDescriptor, { name: 'My prompt' })).toBe(
      'My prompt'
    )
    expect(formSubtitle(memoryDescriptor, { title: 'A memory' })).toBe(
      'A memory'
    )
  })

  it('is undefined with no values, a blank name, or a non-string name', () => {
    expect(formSubtitle(artifactDescriptor, undefined)).toBeUndefined()
    expect(formSubtitle(artifactDescriptor, { title: '   ' })).toBeUndefined()
    expect(formSubtitle(artifactDescriptor, { title: 7 })).toBeUndefined()
  })
})
