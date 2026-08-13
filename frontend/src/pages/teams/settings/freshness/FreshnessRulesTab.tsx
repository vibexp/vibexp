import { Pencil, Plus, RotateCcw, Timer, Trash2 } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EmptyState } from '@/components/EmptyState'
import { LoadingSpinner } from '@/components/LoadingSpinner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { usePermissions } from '@/hooks/usePermissions'
import { toast } from '@/lib/toast'
import type {
  FreshnessRule,
  TeamFreshnessSettings,
} from '@/services/freshnessService'
import { freshnessService } from '@/services/freshnessService'
import type { Project } from '@/services/projectService'
import { projectService } from '@/services/projectService'
import type { Team } from '@/services/teamService'

import {
  DEFAULT_INTERVAL_SECONDS,
  describeInterval,
  describeRule,
  emptyRuleForm,
  formToRequest,
  INTERVAL_PRESETS,
  type RuleFormValues,
  ruleToForm,
} from './freshnessOptions'
import { FreshnessRuleDialog } from './FreshnessRuleDialog'

const errorMessage = (error: unknown, fallback: string): string =>
  error instanceof Error ? error.message : fallback

// ---------------------------------------------------------------------------
// Evaluation card (interval + reversibility)
// ---------------------------------------------------------------------------

interface EvaluationCardProps {
  settings: TeamFreshnessSettings
  intervalSeconds: number
  reversibility: boolean
  canEdit: boolean
  saving: boolean
  resetting: boolean
  hasChanges: boolean
  onIntervalChange: (seconds: number) => void
  onReversibilityChange: (enabled: boolean) => void
  onSave: () => void
  onReset: () => void
}

function EvaluationCard({
  settings,
  intervalSeconds,
  reversibility,
  canEdit,
  saving,
  resetting,
  hasChanges,
  onIntervalChange,
  onReversibilityChange,
  onSave,
  onReset,
}: Readonly<EvaluationCardProps>) {
  const busy = saving || resetting
  const hasOverride = settings.source === 'team'
  // A team whose stored interval came from the API directly may match no
  // preset. Showing it as an extra option preserves it instead of silently
  // rounding the team's configuration to the nearest preset on first save.
  //
  // Keyed on the STORED value, not the current selection: keying on the latter
  // would drop the option the moment the user picked a preset, leaving no
  // control that could restore the team's original cadence.
  const stored = settings.interval_seconds
  const presets = INTERVAL_PRESETS.some(p => p.seconds === stored)
    ? INTERVAL_PRESETS
    : [
        ...INTERVAL_PRESETS,
        { seconds: stored, label: describeInterval(stored) },
      ]

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <CardTitle>Evaluation</CardTitle>
          {settings.source === 'instance' && (
            <Badge variant="secondary">Using instance defaults</Badge>
          )}
        </div>
        <CardDescription>
          How often the rules below run, and whether using a resource clears its
          stale flag.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {!canEdit && (
          <p className="text-muted-foreground text-sm">
            Only team owners and admins can change these settings.
          </p>
        )}

        <fieldset
          className="space-y-3"
          disabled={!canEdit || busy}
          aria-label="Evaluation interval"
        >
          <legend className="text-sm font-medium">Run rules</legend>
          {presets.map(preset => (
            <div key={preset.seconds} className="flex items-center gap-3">
              <input
                type="radio"
                id={`interval-${String(preset.seconds)}`}
                name="freshness-interval"
                value={preset.seconds}
                checked={intervalSeconds === preset.seconds}
                onChange={() => {
                  onIntervalChange(preset.seconds)
                }}
              />
              <Label
                htmlFor={`interval-${String(preset.seconds)}`}
                className="cursor-pointer text-sm font-normal"
              >
                {preset.label}
              </Label>
            </div>
          ))}
        </fieldset>

        <div className="flex items-center justify-between rounded-md border p-3">
          <div className="space-y-0.5">
            <Label htmlFor="reversibility">Clear stale flags on use</Label>
            <p className="text-muted-foreground text-sm">
              When on, opening or editing a stale resource marks it fresh again.
            </p>
          </div>
          <Switch
            id="reversibility"
            checked={reversibility}
            disabled={!canEdit || busy}
            onCheckedChange={onReversibilityChange}
          />
        </div>

        {canEdit && (
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-muted-foreground text-sm">
              {hasOverride
                ? `Reset would restore ${describeInterval(
                    settings.defaults.interval_seconds
                  ).toLowerCase()} evaluation and ${
                    settings.defaults.reversibility_enabled
                      ? 'stale flags cleared on use'
                      : 'stale flags kept until the next run'
                  }.`
                : ''}
            </p>
            <div className="flex gap-2">
              {hasOverride && (
                <Button
                  variant="outline"
                  size="sm"
                  disabled={busy}
                  onClick={onReset}
                >
                  <RotateCcw className="mr-2 size-4" />
                  {resetting ? 'Resetting…' : 'Reset to defaults'}
                </Button>
              )}
              <Button size="sm" disabled={busy || !hasChanges} onClick={onSave}>
                {saving ? 'Saving…' : 'Save evaluation settings'}
              </Button>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}

// ---------------------------------------------------------------------------
// Rules card
// ---------------------------------------------------------------------------

interface RulesCardProps {
  rules: FreshnessRule[]
  projectNames: Map<string, string>
  canEdit: boolean
  onCreate: () => void
  onEdit: (rule: FreshnessRule) => void
  onDelete: (rule: FreshnessRule) => void
}

function RulesCard({
  rules,
  projectNames,
  canEdit,
  onCreate,
  onEdit,
  onDelete,
}: Readonly<RulesCardProps>) {
  if (rules.length === 0) {
    return (
      <EmptyState
        icon={Timer}
        title="No freshness rules"
        description="A rule marks resources stale when nobody has accessed them for a while — for example, artifacts nobody has opened in 90 days. Nothing is flagged until you add one."
        actions={
          canEdit ? (
            <Button onClick={onCreate} data-testid="create-rule-button">
              <Plus className="mr-2 size-4" />
              New rule
            </Button>
          ) : undefined
        }
      />
    )
  }

  return (
    <div className="space-y-3">
      {rules.map(rule => (
        <Card key={rule.id} data-testid="freshness-rule-row">
          <CardContent className="flex flex-wrap items-center justify-between gap-3 p-4">
            <div className="space-y-1">
              <p className="text-sm">
                {describeRule(
                  rule,
                  rule.project_id === null
                    ? undefined
                    : projectNames.get(rule.project_id)
                )}
              </p>
              {!rule.enabled && <Badge variant="outline">Disabled</Badge>}
            </div>
            {canEdit && (
              <div className="flex gap-1">
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="Edit rule"
                  onClick={() => {
                    onEdit(rule)
                  }}
                >
                  <Pencil className="size-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="Delete rule"
                  onClick={() => {
                    onDelete(rule)
                  }}
                >
                  <Trash2 className="size-4" />
                </Button>
              </div>
            )}
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

/**
 * The Settings tab of the Resource Freshness page: evaluation settings plus the
 * rules themselves (#736, epic #726). `FreshnessSettings` is the tab shell that
 * renders this alongside analytics and audit (#737).
 *
 * `team` is the team `TeamScopeLayout` resolved from the URL. Both the
 * permission gating and every read/write MUST key on it rather than on the
 * ambient `currentTeam`: React fires child effects before parent effects, so on
 * a cold deep-link this page's load effect runs BEFORE the layout's
 * `setCurrentTeam` sync and the ambient value is still the previously persisted
 * team (#584).
 *
 * Reads are open to every member — the rules are team-wide policy, so everyone
 * can see what flags their work. Only `team.settings.update` holders get the
 * write affordances, and gating hides them rather than disabling them.
 */
export function FreshnessRulesTab({ team }: Readonly<{ team: Team }>) {
  const { can } = usePermissions(team)
  const canEdit = can('team.settings.update')

  const [settings, setSettings] = useState<TeamFreshnessSettings | null>(null)
  const [rules, setRules] = useState<FreshnessRule[]>([])
  const [projects, setProjects] = useState<Project[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [intervalSeconds, setIntervalSeconds] = useState(
    DEFAULT_INTERVAL_SECONDS
  )
  const [reversibility, setReversibility] = useState(true)
  const [savingSettings, setSavingSettings] = useState(false)
  const [resettingSettings, setResettingSettings] = useState(false)

  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingRule, setEditingRule] = useState<FreshnessRule | null>(null)
  const [ruleForm, setRuleForm] = useState<RuleFormValues>(emptyRuleForm)
  const [savingRule, setSavingRule] = useState(false)
  const [ruleError, setRuleError] = useState<string | null>(null)
  const [ruleToDelete, setRuleToDelete] = useState<FreshnessRule | null>(null)
  const [deleting, setDeleting] = useState(false)

  const teamId = team.id

  const applySettings = (loaded: TeamFreshnessSettings) => {
    setSettings(loaded)
    setIntervalSeconds(loaded.interval_seconds)
    setReversibility(loaded.reversibility_enabled)
  }

  /**
   * The initial load of both surfaces. It owns the full-page spinner, so it
   * runs once on mount and never again — see `refreshRules` for what a write
   * does instead.
   */
  const load = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const [loadedSettings, loadedRules] = await Promise.all([
        freshnessService.getSettings(teamId),
        freshnessService.getRules(teamId),
      ])
      applySettings(loadedSettings)
      setRules(loadedRules)
    } catch (err) {
      setError(errorMessage(err, 'Failed to load freshness settings'))
    } finally {
      setLoading(false)
    }
  }, [teamId])

  /**
   * Refetches only the rules, which is all a rule write can have changed.
   *
   * Deliberately narrower than `load`: re-reading the settings here would call
   * `applySettings` and overwrite the evaluation form, silently discarding an
   * interval or reversibility change the user had made but not yet saved. It
   * also skips the loading flag, so the cards are not unmounted mid-edit.
   */
  const refreshRules = useCallback(async () => {
    try {
      setError(null)
      setRules(await freshnessService.getRules(teamId))
    } catch (err) {
      setError(errorMessage(err, 'Failed to reload the rules'))
    }
  }, [teamId])

  useEffect(() => {
    void load()
  }, [load])

  // Projects only name the rules' scope, so a failure here degrades the rule
  // sentences to "a project" rather than failing the page.
  useEffect(() => {
    let cancelled = false
    projectService
      .getProjects(teamId)
      .then(response => {
        if (!cancelled) setProjects(response.projects)
      })
      .catch((err: unknown) => {
        console.error('Failed to load projects for freshness rules', err)
      })
    return () => {
      cancelled = true
    }
  }, [teamId])

  const handleSaveSettings = async () => {
    try {
      setSavingSettings(true)
      setError(null)
      const updated = await freshnessService.updateSettings(teamId, {
        interval_seconds: intervalSeconds,
        reversibility_enabled: reversibility,
      })
      applySettings(updated)
      toast.success('Freshness settings saved')
    } catch (err) {
      setError(errorMessage(err, 'Failed to save freshness settings'))
    } finally {
      setSavingSettings(false)
    }
  }

  /**
   * Drops the team's override so it inherits the instance defaults again.
   *
   * The refetch is what supplies the restored values, so success is only
   * claimed once it lands — otherwise a failed re-read would show a success
   * toast beside a stale form still displaying the overridden cadence.
   */
  const handleResetSettings = async () => {
    try {
      setResettingSettings(true)
      setError(null)
      await freshnessService.resetSettings(teamId)
      applySettings(await freshnessService.getSettings(teamId))
      toast.success('Reset to the instance defaults')
    } catch (err) {
      setError(errorMessage(err, 'Failed to reset freshness settings'))
    } finally {
      setResettingSettings(false)
    }
  }

  const openCreate = () => {
    setEditingRule(null)
    setRuleForm(emptyRuleForm())
    setRuleError(null)
    setDialogOpen(true)
  }

  const openEdit = (rule: FreshnessRule) => {
    setEditingRule(rule)
    setRuleForm(ruleToForm(rule))
    setRuleError(null)
    setDialogOpen(true)
  }

  const handleSubmitRule = async () => {
    try {
      setSavingRule(true)
      setRuleError(null)
      const request = formToRequest(ruleForm)
      if (editingRule) {
        await freshnessService.updateRule(teamId, editingRule.id, request)
      } else {
        await freshnessService.createRule(teamId, request)
      }
      toast.success(editingRule ? 'Rule updated' : 'Rule created')
      setDialogOpen(false)
      await refreshRules()
    } catch (err) {
      // Kept in the dialog so the user can correct the field that was
      // rejected — closing it would discard everything they typed.
      setRuleError(errorMessage(err, 'Failed to save the rule'))
    } finally {
      setSavingRule(false)
    }
  }

  const handleDeleteRule = async () => {
    if (!ruleToDelete) return
    try {
      setDeleting(true)
      await freshnessService.deleteRule(teamId, ruleToDelete.id)
      toast.success('Rule deleted')
      await refreshRules()
    } catch (err) {
      setError(errorMessage(err, 'Failed to delete the rule'))
    } finally {
      setDeleting(false)
      setRuleToDelete(null)
    }
  }

  // The page title lives on the tab shell, not here — repeating it inside every
  // tab would render it twice under the tab bar.
  if (loading) {
    return (
      <div className="flex justify-center py-12">
        <LoadingSpinner size="lg" />
      </div>
    )
  }

  const errorAlert = error && (
    <Alert variant="destructive">
      <AlertTitle>Error</AlertTitle>
      <AlertDescription>{error}</AlertDescription>
    </Alert>
  )

  if (!settings) {
    return <div className="space-y-6">{errorAlert}</div>
  }

  const projectNames = new Map(projects.map(p => [p.id, p.name]))
  const hasSettingsChanges =
    intervalSeconds !== settings.interval_seconds ||
    reversibility !== settings.reversibility_enabled

  return (
    <div className="space-y-6">
      {errorAlert}

      <EvaluationCard
        settings={settings}
        intervalSeconds={intervalSeconds}
        reversibility={reversibility}
        canEdit={canEdit}
        saving={savingSettings}
        resetting={resettingSettings}
        hasChanges={hasSettingsChanges}
        onIntervalChange={setIntervalSeconds}
        onReversibilityChange={setReversibility}
        onSave={() => {
          void handleSaveSettings()
        }}
        onReset={() => {
          void handleResetSettings()
        }}
      />

      <section className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div>
            <h2 className="text-lg font-semibold">Rules</h2>
            <p className="text-muted-foreground text-sm">
              Every rule is evaluated independently; a resource is stale if any
              rule matches it.
            </p>
          </div>
          {canEdit && rules.length > 0 && (
            <Button onClick={openCreate} data-testid="create-rule-button">
              <Plus className="mr-2 size-4" />
              New rule
            </Button>
          )}
        </div>

        <RulesCard
          rules={rules}
          projectNames={projectNames}
          canEdit={canEdit}
          onCreate={openCreate}
          onEdit={openEdit}
          onDelete={setRuleToDelete}
        />
      </section>

      <FreshnessRuleDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        editing={editingRule !== null}
        form={ruleForm}
        onFormChange={setRuleForm}
        projects={projects}
        submitting={savingRule}
        serverError={ruleError}
        onSubmit={() => {
          void handleSubmitRule()
        }}
      />

      <ConfirmDialog
        open={ruleToDelete !== null}
        onOpenChange={open => {
          if (!open) setRuleToDelete(null)
        }}
        title="Delete this rule?"
        description="Resources flagged only by this rule stop being marked stale. This cannot be undone."
        confirmLabel="Delete"
        variant="destructive"
        loading={deleting}
        onConfirm={handleDeleteRule}
      />
    </div>
  )
}
