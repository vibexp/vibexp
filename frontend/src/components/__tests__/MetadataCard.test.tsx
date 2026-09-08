import { render, screen } from '@testing-library/react'

import { AdditionalDataRows } from '../MetadataCard'

// ---- AdditionalDataRows -----------------------------------------------------

describe('AdditionalDataRows', () => {
  it('returns null when the record is empty', () => {
    const { container } = render(<AdditionalDataRows data={{}} />)
    expect(container.firstChild).toBeNull()
  })

  it('renders string primitives as text', () => {
    render(<AdditionalDataRows data={{ status: 'active' }} />)
    expect(screen.getByText('active')).toBeInTheDocument()
  })

  it('renders boolean true as "Yes"', () => {
    render(<AdditionalDataRows data={{ enabled: true }} />)
    expect(screen.getByText('Yes')).toBeInTheDocument()
  })

  it('renders boolean false as "No"', () => {
    render(<AdditionalDataRows data={{ enabled: false }} />)
    expect(screen.getByText('No')).toBeInTheDocument()
  })

  it('renders numbers via toLocaleString', () => {
    render(<AdditionalDataRows data={{ count: 1000 }} />)
    // toLocaleString('en-US') for 1000 → "1,000" (or "1000" in some locales)
    const rendered = screen.getByText(/1.?000/)
    expect(rendered).toBeInTheDocument()
  })

  it('renders null as em-dash', () => {
    render(<AdditionalDataRows data={{ value: null }} />)
    expect(screen.getByText('—')).toBeInTheDocument()
  })

  it('renders undefined as em-dash', () => {
    render(<AdditionalDataRows data={{ value: undefined }} />)
    expect(screen.getByText('—')).toBeInTheDocument()
  })

  it('renders objects as a JSON code block', () => {
    render(<AdditionalDataRows data={{ nested: { a: 1 } }} />)
    const code = screen.getByText('{"a":1}')
    expect(code.tagName).toBe('CODE')
  })

  it('renders arrays as a JSON code block', () => {
    render(<AdditionalDataRows data={{ items: [1, 2] }} />)
    const code = screen.getByText('[1,2]')
    expect(code.tagName).toBe('CODE')
  })

  it('formats snake_case keys to Sentence case', () => {
    render(<AdditionalDataRows data={{ active_count: 5 }} />)
    expect(screen.getByText('Active count')).toBeInTheDocument()
  })

  it('formats kebab-case keys to Sentence case', () => {
    render(<AdditionalDataRows data={{ 'my-key': 'val' }} />)
    expect(screen.getByText('My key')).toBeInTheDocument()
  })

  it('renders "[unserializable]" for circular reference objects', () => {
    // Simulate a value that cannot be JSON.stringified
    const circular: Record<string, unknown> = {}
    circular.self = circular
    render(<AdditionalDataRows data={{ ref: circular }} />)
    expect(screen.getByText('[unserializable]')).toBeInTheDocument()
  })
})
