import { AlertCircle, CheckCircle2 } from 'lucide-react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import type { TeamEmailProviderTestResponse } from '@/services/emailProviderService'

/**
 * Outcome of a test send — reported inline, never as a thrown error: a
 * rejected send comes back 200 with `is_valid: false`. Shared by the team
 * email-provider page and Admin → Settings → Email (#1191), whose test
 * endpoints return the same response type. `note` adds a caller's caveat about
 * what the test did or did not exercise (#1222).
 */
export function TestResultAlert({
  result,
  testId,
  note,
}: Readonly<{
  result: TeamEmailProviderTestResponse
  testId?: string
  note?: string
}>) {
  return (
    <Alert
      variant={result.is_valid ? 'default' : 'destructive'}
      data-testid={testId}
    >
      {result.is_valid ? (
        <CheckCircle2 className="size-4" />
      ) : (
        <AlertCircle className="size-4" />
      )}
      <AlertTitle>
        {result.is_valid ? 'Test email sent' : 'Test email failed'}
      </AlertTitle>
      <AlertDescription>
        <p>{result.message}</p>
        <p className="mt-1">Sent to {result.recipient}.</p>
        {result.details.error_details && (
          <p className="mt-1">Reason: {result.details.error_details}</p>
        )}
        {note && <p className="mt-1">{note}</p>}
      </AlertDescription>
    </Alert>
  )
}
