import { CopyButton } from '@/components/CopyButton'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'

/**
 * A one-time account link, shown once (#1443): the API returns it a single
 * time and the page keeps it in component state only, so closing the panel
 * loses it. A new link can be made from the user page.
 */
export function LinkReveal({
  link,
  title = 'Copy this link now',
}: Readonly<{ link: string; title?: string }>) {
  return (
    <Alert data-testid="link-reveal">
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription className="space-y-2">
        <div className="bg-muted flex items-start gap-2 rounded-md p-2">
          <code
            className="min-w-0 flex-1 font-mono text-xs break-all"
            data-testid="link-reveal-value"
          >
            {link}
          </code>
          <CopyButton value={link} label="Copy link" />
        </div>
        <p>
          It will not be shown again. Anyone with the link can set the password,
          so send it only to the person it is for.
        </p>
      </AlertDescription>
    </Alert>
  )
}
