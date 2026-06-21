import { render, screen } from '@testing-library/react'

import { AdditionalDataCard } from '../MetadataCard'

// ---- AdditionalDataCard -----------------------------------------------------

describe('AdditionalDataCard', () => {
  it('returns null when the record is empty', () => {
    const { container } = render(<AdditionalDataCard data={{}} />)
    expect(container.firstChild).toBeNull()
  })

  it('renders string primitives as text', () => {
    render(<AdditionalDataCard data={{ status: 'active' }} />)
    expect(screen.getByText('active')).toBeInTheDocument()
  })

  it('renders boolean true as "Yes"', () => {
    render(<AdditionalDataCard data={{ enabled: true }} />)
    expect(screen.getByText('Yes')).toBeInTheDocument()
  })

  it('renders boolean false as "No"', () => {
    render(<AdditionalDataCard data={{ enabled: false }} />)
    expect(screen.getByText('No')).toBeInTheDocument()
  })

  it('renders numbers via toLocaleString', () => {
    render(<AdditionalDataCard data={{ count: 1000 }} />)
    // toLocaleString('en-US') for 1000 → "1,000" (or "1000" in some locales)
    const rendered = screen.getByText(/1.?000/)
    expect(rendered).toBeInTheDocument()
  })

  it('renders null as em-dash', () => {
    render(<AdditionalDataCard data={{ value: null }} />)
    expect(screen.getByText('—')).toBeInTheDocument()
  })

  it('renders undefined as em-dash', () => {
    render(<AdditionalDataCard data={{ value: undefined }} />)
    expect(screen.getByText('—')).toBeInTheDocument()
  })

  it('renders objects as a JSON code block', () => {
    render(<AdditionalDataCard data={{ nested: { a: 1 } }} />)
    const code = screen.getByText('{"a":1}')
    expect(code.tagName).toBe('CODE')
  })

  it('renders arrays as a JSON code block', () => {
    render(<AdditionalDataCard data={{ items: [1, 2] }} />)
    const code = screen.getByText('[1,2]')
    expect(code.tagName).toBe('CODE')
  })

  it('formats snake_case keys to Sentence case', () => {
    render(<AdditionalDataCard data={{ active_count: 5 }} />)
    expect(screen.getByText('Active count')).toBeInTheDocument()
  })

  it('formats kebab-case keys to Sentence case', () => {
    render(<AdditionalDataCard data={{ 'my-key': 'val' }} />)
    expect(screen.getByText('My key')).toBeInTheDocument()
  })

  it('renders the "Additional data" heading', () => {
    render(<AdditionalDataCard data={{ foo: 'bar' }} />)
    expect(screen.getByText('Additional data')).toBeInTheDocument()
  })

  it('renders "[unserializable]" for circular reference objects', () => {
    // Simulate a value that cannot be JSON.stringified
    const circular: Record<string, unknown> = {}
    circular.self = circular
    render(<AdditionalDataCard data={{ ref: circular }} />)
    expect(screen.getByText('[unserializable]')).toBeInTheDocument()
  })
})
