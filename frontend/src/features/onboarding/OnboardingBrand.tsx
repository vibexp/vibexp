import { Logo } from '@vibexp/design-system/react'

import { BrandLogo } from '@/features/site-info/BrandLogo'
import { useBrandLockup } from '@/features/site-info/useBrandLockup'
import { useSiteInfo } from '@/features/site-info/useSiteInfo'

const LOGO_SIZE = 26

/**
 * The onboarding header's logo. Unbranded, it is the design-system `Logo`
 * lockup the design shows. With company branding (epic #1352) it is the
 * company's logo and name, as in the app's sidebar, decided by the same
 * `useBrandLockup` rule.
 */
export function OnboardingBrand() {
  const { company_name } = useSiteInfo()
  const { logo, name, markIsDecorative } = useBrandLockup()

  if (company_name === null && logo.src === null) {
    return <Logo size={LOGO_SIZE} />
  }

  return (
    <span className="flex items-center gap-2.5">
      <BrandLogo
        size={LOGO_SIZE}
        source={logo}
        decorative={markIsDecorative}
        className="max-w-40"
      />
      {name !== null && (
        <span className="truncate text-base font-semibold tracking-[-0.018em]">
          {name}
        </span>
      )}
    </span>
  )
}
