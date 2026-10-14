import { Link } from 'react-router'

import { Button } from '@/components/ui/button'
import { useSiteInfo } from '@/features/site-info/useSiteInfo'

/**
 * Small pieces for the public cards a person reaches from an email (#1473):
 * where they are, and a way on from a dead end.
 */

/**
 * The instance's name at the top of a card. A branded instance already shows
 * its lockup above the card (`PublicPageBrand`), so this is for the others.
 */
export function InstanceName() {
  const { displayName, hasCompanyBranding } = useSiteInfo()
  if (hasCompanyBranding) return null
  return (
    <p
      className="text-muted-foreground text-sm font-medium"
      data-testid="public-instance-name"
    >
      {displayName}
    </p>
  )
}

/** The way on from a link that cannot be used. */
export function GoToSignIn() {
  return (
    <Button asChild variant="outline">
      <Link to="/login">Go to sign in</Link>
    </Button>
  )
}
