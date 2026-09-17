import { render } from '@testing-library/react'

import { GitHubAppSetupGuide } from './GitHubAppSetupGuide'

// JSX collapses a newline between `</strong>` and adjacent text to nothing, so
// the rendered sentence hangs on how the source happens to be wrapped (#1029).
// Assert the joined text rather than the markup: a reformat that drops or
// doubles the space around the period is exactly the regression this guards.
it('renders the user-authorization sentence with correct spacing around the bold phrase', () => {
  const { container } = render(<GitHubAppSetupGuide teamId="team-1" />)

  const paragraph = Array.from(container.querySelectorAll('p')).find(p =>
    p.textContent.includes('This is not optional')
  )

  expect(paragraph?.textContent.replace(/\s+/g, ' ')).toContain(
    'Enable “Request user authorization (OAuth) during installation”. This is not optional:'
  )
})
