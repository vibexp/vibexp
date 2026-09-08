import { render, screen } from '@testing-library/react'

import { TaxonomyChips } from '../TaxonomyChips'

describe('TaxonomyChips', () => {
  it('renders nothing for an empty list', () => {
    const { container } = render(<TaxonomyChips values={[]} />)
    expect(container.firstChild).toBeNull()
  })

  it('renders a single value as one chip', () => {
    render(<TaxonomyChips values={['code-review']} />)
    expect(screen.getByText('code-review')).toBeInTheDocument()
  })

  it('renders every value, in order', () => {
    const { container } = render(
      <TaxonomyChips values={['alpha', 'beta', 'gamma']} />
    )
    expect(
      Array.from(container.firstElementChild?.children ?? []).map(
        chip => chip.textContent
      )
    ).toEqual(['alpha', 'beta', 'gamma'])
  })

  it('gives every chip the one shared badge treatment', () => {
    // Prompt labels used `outline` and memory tags used `secondary` + an icon
    // before #904; drift here is the whole point of the component.
    render(<TaxonomyChips values={['alpha', 'beta']} />)
    const classes = ['alpha', 'beta'].map(
      value => screen.getByText(value).className
    )
    expect(classes[0]).toBe(classes[1])
    expect(classes[0]).toContain('bg-secondary')
  })
})
