import { zodResolver } from '@hookform/resolvers/zod'
import { Network, Sparkles, Terminal } from 'lucide-react'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

export const AVAILABLE_INTEGRATIONS = [
  {
    code: 'ai_tools',
    name: 'AI Tools',
    description:
      'Use with Claude Code, Cursor IDE, and other AI-powered development tools.',
    icon: Sparkles,
  },
  {
    code: 'cli',
    name: 'VibeXP CLI',
    description:
      'Access VibeXP from command line for automation and scripting.',
    icon: Terminal,
  },
  {
    code: 'mcp_server',
    name: 'MCP Server',
    description: 'Connect via Model Context Protocol for AI assistants.',
    icon: Network,
  },
] as const

const schema = z.object({
  name: z
    .string()
    .trim()
    .min(1, 'Name is required')
    .max(255, 'Name must be under 255 characters'),
  integrations: z
    .array(z.enum(['ai_tools', 'cli', 'mcp_server']))
    .min(1, 'Select at least one integration'),
})

export type CreateAPIKeyFormValues = z.infer<typeof schema>

interface CreateAPIKeyDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  submitting: boolean
  onSubmit: (
    values: CreateAPIKeyFormValues,
    setFieldError: (field: 'name' | 'integrations', message: string) => void
  ) => void | Promise<void>
}

export function CreateAPIKeyDialog({
  open,
  onOpenChange,
  submitting,
  onSubmit,
}: Readonly<CreateAPIKeyDialogProps>) {
  const form = useForm<CreateAPIKeyFormValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: '', integrations: [] },
  })

  useEffect(() => {
    if (!open) form.reset()
  }, [open, form])

  const handleSubmit = form.handleSubmit(values => {
    return onSubmit(values, (field, message) => {
      form.setError(field, { message })
    })
  })

  // Disable submit until the form is minimally valid: a name and at least one
  // integration are both required.
  const nameValue = form.watch('name')
  const integrationValues = form.watch('integrations')
  const canSubmit = nameValue.trim().length > 0 && integrationValues.length > 0

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Create API key</DialogTitle>
          <DialogDescription>
            Give your key a descriptive name and pick the integrations where it
            will be used.
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form
            data-testid="create-api-key-form"
            onSubmit={event => {
              void handleSubmit(event)
            }}
            className="space-y-4"
          >
            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Name</FormLabel>
                  <FormControl>
                    <Input
                      data-testid="api-key-name-input"
                      placeholder="e.g., Development Setup"
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="integrations"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Integrations</FormLabel>
                  <div className="space-y-2">
                    {AVAILABLE_INTEGRATIONS.map(integration => {
                      const Icon = integration.icon
                      const checked = field.value.includes(integration.code)
                      return (
                        <label
                          key={integration.code}
                          htmlFor={`integration-${integration.code}`}
                          className={cn(
                            'flex cursor-pointer items-start gap-3 rounded-md border p-3 transition-colors',
                            checked
                              ? 'border-primary bg-primary/5'
                              : 'hover:bg-muted/50'
                          )}
                        >
                          <Checkbox
                            id={`integration-${integration.code}`}
                            data-testid={`integration-checkbox-${integration.code}`}
                            checked={checked}
                            onCheckedChange={value => {
                              const next = value === true
                              const current = field.value
                              field.onChange(
                                next
                                  ? [...current, integration.code]
                                  : current.filter(c => c !== integration.code)
                              )
                            }}
                            className="mt-0.5"
                          />
                          <div className="flex-1 space-y-0.5">
                            <div className="flex items-center gap-2 text-sm font-medium">
                              <Icon className="size-4" />
                              {integration.name}
                            </div>
                            <p className="text-muted-foreground text-xs">
                              {integration.description}
                            </p>
                          </div>
                        </label>
                      )
                    })}
                  </div>
                  <FormMessage />
                </FormItem>
              )}
            />
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                disabled={submitting}
                onClick={() => {
                  onOpenChange(false)
                }}
              >
                Cancel
              </Button>
              <Button
                type="submit"
                disabled={submitting || !canSubmit}
                data-testid="submit-create-api-key-button"
              >
                {submitting ? 'Creating…' : 'Create API key'}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
