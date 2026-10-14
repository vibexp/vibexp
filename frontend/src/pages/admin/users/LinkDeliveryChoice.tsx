import { Link } from 'react-router'

import { Button } from '@/components/ui/button'

export type LinkDelivery = 'email' | 'link'

/**
 * The two ways an administrator hands over a setup or reset link (#1443):
 * email it, or show it here once. With no email provider configured
 * (`emailLinks` false) emailing cannot work, so it is disabled with the
 * reason and showing the link becomes the main action (#1494). While unknown
 * (null) both are offered and the server's `email_unavailable` answer stays
 * the fallback, since email can also be removed while the dialog is open.
 */
export function LinkDeliveryChoice({
  email,
  emailLinks,
  busy,
  onChoose,
}: Readonly<{
  email: string
  emailLinks: boolean | null
  busy: boolean
  onChoose: (delivery: LinkDelivery) => void
}>) {
  const noEmail = emailLinks === false
  const emailButton = (
    <Button
      type="button"
      variant={noEmail ? 'outline' : 'default'}
      disabled={busy || noEmail}
      // A long address must not widen the dialog past the screen: the label
      // truncates, and the full address is in the title.
      className="min-w-0"
      title={`Email it to ${email}`}
      onClick={() => {
        onChoose('email')
      }}
    >
      <span className="truncate">Email it to {email}</span>
    </Button>
  )
  const showButton = (
    <Button
      type="button"
      variant={noEmail ? 'default' : 'outline'}
      disabled={busy}
      onClick={() => {
        onChoose('link')
      }}
    >
      Show the link here
    </Button>
  )
  return (
    <div className="min-w-0 space-y-2">
      <div className="flex min-w-0 flex-col gap-2 sm:flex-row">
        {noEmail ? (
          <>
            {showButton}
            {emailButton}
          </>
        ) : (
          <>
            {emailButton}
            {showButton}
          </>
        )}
      </div>
      {noEmail && (
        <p
          className="text-muted-foreground text-sm"
          data-testid="link-delivery-no-email"
        >
          No email provider is configured, so the link cannot be emailed: show
          it here and hand it over.{' '}
          <Link
            to="/admin/settings/email"
            className="font-medium underline underline-offset-4"
          >
            Set up email
          </Link>
        </p>
      )}
    </div>
  )
}
