import { AlertCircle, Loader2 } from 'lucide-react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { PublicCardPage } from '@/features/site-info/PublicPageBrand'

import { GoToSignIn } from './PublicCardParts'

/**
 * The cards the one-time account link pages share (setup #1440, reset #1439):
 * the waiting card and the common notices. Reading a failure is in
 * accountLinkFailure.ts.
 */

export function CheckingLink() {
  return (
    <PublicCardPage>
      <div className="flex flex-col items-center gap-3 text-center">
        <Loader2 className="text-primary size-10 animate-spin" />
        <p className="text-muted-foreground text-sm">Checking your link…</p>
      </div>
    </PublicCardPage>
  )
}

export function LinkNotice({
  title,
  text,
}: Readonly<{ title: string; text: string }>) {
  return (
    <Alert variant="destructive">
      <AlertCircle className="size-4" />
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>{text}</AlertDescription>
    </Alert>
  )
}

/** The password was stored but signing in did not finish (a 500). */
export function SetButNotSignedIn({
  title,
  email,
}: Readonly<{ title: string; email: string }>) {
  return (
    <>
      <Alert>
        <AlertTitle>{title}</AlertTitle>
        <AlertDescription>
          Signing you in did not finish. Sign in with {email} and the password
          you just chose.
        </AlertDescription>
      </Alert>
      <GoToSignIn variant="default" />
    </>
  )
}
