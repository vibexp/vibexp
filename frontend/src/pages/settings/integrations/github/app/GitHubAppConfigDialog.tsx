import { zodResolver } from '@hookform/resolvers/zod'
import { Loader2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/lib/toast'
import type {
  CreateGitHubAppConfigRequest,
  GitHubAppConfigResponse,
  UpdateGitHubAppConfigRequest,
} from '@/services/githubAppConfigService'
import { githubAppConfigService } from '@/services/githubAppConfigService'

import { describeValidationFailure } from './validationMessages'

const KEEP_CURRENT = 'Leave blank to keep the current value'

const schema = z.object({
  app_id: z.string().trim().min(1, 'App ID is required').max(50),
  app_slug: z.string().trim().min(1, 'App slug is required').max(255),
  client_id: z.string().trim().min(1, 'Client ID is required').max(255),
  client_secret: z.string().optional(),
  private_key: z.string().optional(),
})

export type GitHubAppConfigFormValues = z.infer<typeof schema>

export interface GitHubAppConfigDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  teamId: string
  /** Existing registration when editing; undefined when registering a new App. */
  config?: GitHubAppConfigResponse
  /**
   * Called after a successful save. `webhookSecret` is present only on create —
   * it is the one and only disclosure of the generated secret.
   */
  onSaved: (result: {
    webhookUrl: string
    webhookSecret?: string
  }) => void | Promise<void>
}

/**
 * The credential form.
 *
 * Two behaviours are load-bearing and easy to get wrong:
 *
 *  - **Blank secrets on edit mean "keep the stored one".** The UI never receives
 *    the current secrets, so it cannot resend them; an empty string would be a
 *    validation error server-side, not a silent clear. Blank fields are
 *    therefore submitted as `undefined`, never `''`.
 *  - **A failed validate probe is reported on the field that is wrong.** The
 *    server's categories are coarse by design (#464), so mapping them to a
 *    specific input is what makes them useful.
 */
export function GitHubAppConfigDialog({
  open,
  onOpenChange,
  teamId,
  config,
  onSaved,
}: GitHubAppConfigDialogProps) {
  const isEdit = Boolean(config)
  const [submitting, setSubmitting] = useState(false)

  const form = useForm<GitHubAppConfigFormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      app_id: '',
      app_slug: '',
      client_id: '',
      client_secret: '',
      private_key: '',
    },
  })

  useEffect(() => {
    if (!open) {
      // Clearing on close is what keeps a pasted private key from lingering in
      // component state after the dialog is done with it.
      form.reset()
      return
    }
    form.reset({
      app_id: config?.app_id ?? '',
      app_slug: config?.app_slug ?? '',
      client_id: config?.client_id ?? '',
      client_secret: '',
      private_key: '',
    })
  }, [open, config, form])

  const handleSubmit = form.handleSubmit(async values => {
    const clientSecret = values.client_secret?.trim() ?? ''
    const privateKey = values.private_key?.trim() ?? ''

    if (!isEdit && (clientSecret === '' || privateKey === '')) {
      if (clientSecret === '') {
        form.setError('client_secret', { message: 'Client secret is required' })
      }
      if (privateKey === '') {
        form.setError('private_key', { message: 'Private key is required' })
      }
      return
    }

    setSubmitting(true)
    try {
      let webhookUrl: string
      let webhookSecret: string | undefined

      if (isEdit) {
        const request: UpdateGitHubAppConfigRequest = {
          app_id: values.app_id.trim(),
          app_slug: values.app_slug.trim(),
          client_id: values.client_id.trim(),
          // undefined, never '' — an explicit empty is a server-side
          // validation error, not "clear this secret".
          client_secret: clientSecret === '' ? undefined : clientSecret,
          private_key: privateKey === '' ? undefined : privateKey,
        }
        const updated = await githubAppConfigService.updateAppConfig(
          teamId,
          request
        )
        webhookUrl = updated.webhook_url
      } else {
        const request: CreateGitHubAppConfigRequest = {
          app_id: values.app_id.trim(),
          app_slug: values.app_slug.trim(),
          client_id: values.client_id.trim(),
          client_secret: clientSecret,
          private_key: privateKey,
        }
        const created = await githubAppConfigService.createAppConfig(
          teamId,
          request
        )
        webhookUrl = created.webhook_url
        webhookSecret = created.webhook_secret
      }

      // Validate-on-save: prove the stored credentials actually work before
      // telling anyone setup succeeded. A failed probe is reported on the field
      // at fault and the dialog stays open.
      const validation = await githubAppConfigService.validateAppConfig(teamId)
      if (!validation.is_valid) {
        const failure = describeValidationFailure(
          validation.details?.error_details,
          validation.message
        )
        if (failure.field && failure.field in form.getValues()) {
          form.setError(failure.field, {
            message: failure.description,
          })
        } else {
          toast.error(failure.title, { description: failure.description })
        }
        return
      }

      form.reset()
      onOpenChange(false)
      await onSaved({ webhookUrl, webhookSecret })
    } catch (error) {
      toast.error(
        isEdit
          ? 'Could not update the GitHub App'
          : 'Could not register the GitHub App',
        {
          description:
            error instanceof Error ? error.message : 'Please try again.',
        }
      )
    } finally {
      setSubmitting(false)
    }
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {isEdit ? 'Edit GitHub App' : 'Register GitHub App'}
          </DialogTitle>
          <DialogDescription>
            Copy these from your GitHub App’s settings page. Secrets are stored
            encrypted and never shown again.
          </DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form
            onSubmit={event => {
              void handleSubmit(event)
            }}
            className="space-y-4"
          >
            <FormField
              control={form.control}
              name="app_id"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>App ID</FormLabel>
                  <FormControl>
                    <Input placeholder="123456" {...field} />
                  </FormControl>
                  <FormDescription>
                    The numeric ID at the top of the App’s General page.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="app_slug"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>App slug</FormLabel>
                  <FormControl>
                    <Input placeholder="my-vibexp-app" {...field} />
                  </FormControl>
                  <FormDescription>
                    The last segment of github.com/apps/&lt;slug&gt; — not the
                    display name.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="client_id"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Client ID</FormLabel>
                  <FormControl>
                    <Input placeholder="Iv1.abc123" {...field} />
                  </FormControl>
                  <FormDescription>
                    Not a secret — GitHub shows it on the App settings page.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="client_secret"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Client secret</FormLabel>
                  <FormControl>
                    <Input
                      type="password"
                      autoComplete="off"
                      placeholder={isEdit ? KEEP_CURRENT : ''}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    Generated on the App’s General page and shown once by
                    GitHub.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="private_key"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Private key</FormLabel>
                  <FormControl>
                    <Textarea
                      rows={6}
                      autoComplete="off"
                      spellCheck={false}
                      className="font-mono text-xs"
                      placeholder={
                        isEdit
                          ? KEEP_CURRENT
                          : '-----BEGIN RSA PRIVATE KEY-----'
                      }
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    Paste the .pem GitHub downloads. Raw PEM or base64 — both
                    are accepted.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  onOpenChange(false)
                }}
                disabled={submitting}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={submitting}>
                {submitting ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Validating…
                  </>
                ) : isEdit ? (
                  'Save changes'
                ) : (
                  'Register App'
                )}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
