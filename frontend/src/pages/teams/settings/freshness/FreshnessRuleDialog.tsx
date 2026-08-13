import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import type { Project } from '@/services/projectService'

import {
  ANY_PROJECT,
  MEDIUM_OPTIONS,
  RESOURCE_TYPE_OPTIONS,
  type RuleFormValues,
  validateRule,
} from './freshnessOptions'

// Matches `RelationComposer`'s native select — the app has no Select-free
// styled primitive, and a Radix `Select` must not be used here: inside a Dialog
// it drives jsdom into an infinite focus-scope loop that OOMs the whole test
// suite. Checkboxes and a native select keep this form testable.
const selectClass =
  'border-input bg-background focus-visible:ring-ring h-9 w-full rounded-md border px-3 py-1 text-sm focus-visible:ring-1 focus-visible:outline-none'

interface FreshnessRuleDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Present when editing; absent when creating. Drives the copy only. */
  editing: boolean
  form: RuleFormValues
  onFormChange: (form: RuleFormValues) => void
  projects: Project[]
  submitting: boolean
  /** A server-side rejection, surfaced verbatim beside the local validation. */
  serverError: string | null
  onSubmit: () => void
}

/**
 * Create/edit form for one freshness rule.
 *
 * The form state lives in the page rather than here, so the page can seed it
 * from the rule being edited and keep it across a failed submit — the dialog is
 * a controlled presentation of `form`.
 */
export function FreshnessRuleDialog({
  open,
  onOpenChange,
  editing,
  form,
  onFormChange,
  projects,
  submitting,
  serverError,
  onSubmit,
}: Readonly<FreshnessRuleDialogProps>) {
  const validationError = validateRule(form)

  const toggleResourceType = (
    value: (typeof RESOURCE_TYPE_OPTIONS)[number]['value']
  ) => {
    const next = form.resourceTypes.includes(value)
      ? form.resourceTypes.filter(t => t !== value)
      : [...form.resourceTypes, value]
    onFormChange({ ...form, resourceTypes: next })
  }

  const toggleMedium = (value: (typeof MEDIUM_OPTIONS)[number]['value']) => {
    const next = form.mediums.includes(value)
      ? form.mediums.filter(m => m !== value)
      : [...form.mediums, value]
    onFormChange({ ...form, mediums: next })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{editing ? 'Edit rule' : 'New rule'}</DialogTitle>
          <DialogDescription>
            A rule marks resources stale when nobody has accessed them for the
            threshold you set.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <fieldset className="space-y-2" disabled={submitting}>
            <legend className="text-sm font-medium">Resource types</legend>
            <p className="text-muted-foreground text-xs">
              The rule applies to every type you select.
            </p>
            <div className="grid grid-cols-2 gap-2">
              {RESOURCE_TYPE_OPTIONS.map(option => (
                <div key={option.value} className="flex items-center gap-2">
                  <Checkbox
                    id={`resource-type-${option.value}`}
                    checked={form.resourceTypes.includes(option.value)}
                    onCheckedChange={() => {
                      toggleResourceType(option.value)
                    }}
                  />
                  <Label
                    htmlFor={`resource-type-${option.value}`}
                    className="cursor-pointer text-sm font-normal"
                  >
                    {option.label}
                  </Label>
                </div>
              ))}
            </div>
          </fieldset>

          <div className="space-y-1">
            <Label htmlFor="rule-project">Project</Label>
            <select
              id="rule-project"
              className={selectClass}
              value={form.projectId}
              disabled={submitting}
              onChange={e => {
                onFormChange({ ...form, projectId: e.target.value })
              }}
              data-testid="rule-project-select"
            >
              <option value={ANY_PROJECT}>Any project</option>
              {projects.map(project => (
                <option key={project.id} value={project.id}>
                  {project.name}
                </option>
              ))}
            </select>
          </div>

          <fieldset className="space-y-2" disabled={submitting}>
            <legend className="text-sm font-medium">Access mediums</legend>
            <p className="text-muted-foreground text-xs">
              Which kinds of access keep a resource fresh. Select none to count
              any medium.
            </p>
            <div className="flex flex-wrap gap-4">
              {MEDIUM_OPTIONS.map(option => (
                <div key={option.value} className="flex items-center gap-2">
                  <Checkbox
                    id={`medium-${option.value}`}
                    checked={form.mediums.includes(option.value)}
                    onCheckedChange={() => {
                      toggleMedium(option.value)
                    }}
                  />
                  <Label
                    htmlFor={`medium-${option.value}`}
                    className="cursor-pointer text-sm font-normal"
                  >
                    {option.label}
                  </Label>
                </div>
              ))}
            </div>
          </fieldset>

          <div className="space-y-1">
            <Label htmlFor="rule-threshold">Threshold (days)</Label>
            <Input
              id="rule-threshold"
              type="number"
              min={1}
              value={form.thresholdDays}
              disabled={submitting}
              onChange={e => {
                onFormChange({ ...form, thresholdDays: e.target.value })
              }}
            />
            <p className="text-muted-foreground text-xs">
              Days without a qualifying access before the resource is marked
              stale.
            </p>
          </div>

          <div className="flex items-center justify-between rounded-md border p-3">
            <div className="space-y-0.5">
              <Label htmlFor="rule-enabled">Enabled</Label>
              <p className="text-muted-foreground text-xs">
                Disabled rules are kept but skipped by evaluation runs.
              </p>
            </div>
            <Switch
              id="rule-enabled"
              checked={form.enabled}
              disabled={submitting}
              onCheckedChange={checked => {
                onFormChange({ ...form, enabled: checked })
              }}
            />
          </div>

          {serverError && (
            <Alert variant="destructive">
              <AlertTitle>Could not save the rule</AlertTitle>
              <AlertDescription>{serverError}</AlertDescription>
            </Alert>
          )}

          {/* Above the footer, so the reason the submit button is disabled
              reads before the button itself. */}
          {validationError && (
            <p className="text-destructive text-sm" role="alert">
              {validationError}
            </p>
          )}
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            disabled={submitting}
            onClick={() => {
              onOpenChange(false)
            }}
          >
            Cancel
          </Button>
          <Button
            disabled={submitting || validationError !== null}
            onClick={onSubmit}
            data-testid="submit-rule-button"
          >
            {submitting ? 'Saving…' : editing ? 'Save rule' : 'Create rule'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
