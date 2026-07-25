import { ChevronDown, Info, RotateCcw } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import { PageHeader } from '@/components/PageHeader'
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
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useTeam } from '@/contexts/TeamContext'
import { usePermissions } from '@/hooks/usePermissions'
import {
  searchSettingsService,
  type TeamSearchSettings,
} from '@/services/searchSettingsService'

import {
  describeValues,
  detectPreset,
  effectiveValues,
  NUMERIC_FIELDS,
  type NumericField,
  RANKING_PRESETS,
  type RankingForm,
  type RankingPresetId,
  sameValues,
  toForm,
  toValues,
  validate,
} from './searchRankingPresets'

// ---------------------------------------------------------------------------
// Ranking-profile card
// ---------------------------------------------------------------------------

interface ProfileCardProps {
  selectedPreset: RankingPresetId
  canEdit: boolean
  busy: boolean
  isInherited: boolean
  onSelect: (presetId: RankingPresetId) => void
}

function ProfileCard({
  selectedPreset,
  canEdit,
  busy,
  isInherited,
  onSelect,
}: Readonly<ProfileCardProps>) {
  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <CardTitle>Ranking profile</CardTitle>
          {isInherited && (
            <Badge variant="secondary">Using instance defaults</Badge>
          )}
        </div>
        <CardDescription>
          Applies to everyone in this team, in the web app and in any
          MCP-connected AI tool.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {!canEdit && (
          <p className="text-muted-foreground text-sm">
            Only team owners and admins can change these settings.
          </p>
        )}

        <fieldset
          className="space-y-3"
          disabled={!canEdit || busy}
          aria-label="Ranking profile"
        >
          {RANKING_PRESETS.map(preset => (
            <div
              key={preset.id}
              className="flex items-start gap-3 rounded-md border p-3"
            >
              <input
                type="radio"
                id={`preset-${preset.id}`}
                name="ranking-preset"
                className="mt-1"
                value={preset.id}
                checked={selectedPreset === preset.id}
                onChange={() => {
                  onSelect(preset.id)
                }}
              />
              <div className="space-y-1">
                <Label
                  htmlFor={`preset-${preset.id}`}
                  className="block cursor-pointer text-sm font-medium"
                >
                  {preset.label}
                </Label>
                <p className="text-muted-foreground text-sm">
                  {preset.description}
                </p>
              </div>
            </div>
          ))}

          {/* Custom is derived, never chosen: it is what the profile *is* when
              the numbers match no preset. Selecting it directly would mean
              nothing, so this radio only reports state. */}
          <div className="flex items-start gap-3 rounded-md border border-dashed p-3">
            <input
              type="radio"
              id="preset-custom"
              name="ranking-preset"
              className="mt-1"
              value="custom"
              checked={selectedPreset === 'custom'}
              readOnly
              disabled
            />
            <div className="space-y-1">
              <Label
                htmlFor="preset-custom"
                className="block text-sm font-medium"
              >
                Custom
              </Label>
              <p className="text-muted-foreground text-sm">
                Selected automatically when the weights below match no preset.
              </p>
            </div>
          </div>
        </fieldset>
      </CardContent>
    </Card>
  )
}

// ---------------------------------------------------------------------------
// Advanced disclosure
// ---------------------------------------------------------------------------

interface AdvancedCardProps {
  form: RankingForm
  candidateCap: number
  canEdit: boolean
  busy: boolean
  validationError: string | null
  onNumericChange: (key: NumericField, value: string) => void
}

function AdvancedCard({
  form,
  candidateCap,
  canEdit,
  busy,
  validationError,
  onNumericChange,
}: Readonly<AdvancedCardProps>) {
  const [open, setOpen] = useState(false)

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <Card>
        <CardHeader>
          <CollapsibleTrigger className="flex w-full items-center justify-between gap-2 text-left">
            <span>
              <CardTitle>Advanced</CardTitle>
              <CardDescription className="mt-1">
                Raw weights and decay half-life. The three weights are
                normalized by their sum, so only their ratio matters.
              </CardDescription>
            </span>
            <ChevronDown
              className={`text-muted-foreground size-4 shrink-0 transition-transform ${
                open ? 'rotate-180' : ''
              }`}
            />
          </CollapsibleTrigger>
        </CardHeader>
        <CollapsibleContent>
          <CardContent className="space-y-4">
            {form.recency_ranking_enabled ? (
              <Alert>
                <Info className="size-4" />
                <AlertTitle>Pagination is capped</AlertTitle>
                <AlertDescription>
                  With recency ranking enabled, search re-ranks the top{' '}
                  {candidateCap} matches — results beyond that aren&apos;t
                  reachable via pagination.
                </AlertDescription>
              </Alert>
            ) : (
              <p className="text-muted-foreground text-sm">
                Recency ranking is off, so these values are stored but do not
                affect ordering. They are kept so switching a recency preset
                back on restores your tuning.
              </p>
            )}

            <fieldset
              className="grid gap-4 sm:grid-cols-2"
              disabled={!canEdit || busy}
            >
              {NUMERIC_FIELDS.map(field => (
                <div key={field.key} className="space-y-1">
                  <Label htmlFor={field.key}>{field.label}</Label>
                  <Input
                    id={field.key}
                    type="number"
                    min={0}
                    step={field.step}
                    value={form[field.key]}
                    onChange={e => {
                      onNumericChange(field.key, e.target.value)
                    }}
                  />
                  <p className="text-muted-foreground text-xs">{field.hint}</p>
                </div>
              ))}
            </fieldset>

            {validationError && (
              <Alert variant="destructive">
                <AlertTitle>Invalid values</AlertTitle>
                <AlertDescription>{validationError}</AlertDescription>
              </Alert>
            )}
          </CardContent>
        </CollapsibleContent>
      </Card>
    </Collapsible>
  )
}

// ---------------------------------------------------------------------------
// Save / reset footer
// ---------------------------------------------------------------------------

interface ActionFooterProps {
  settings: TeamSearchSettings
  hasChanges: boolean
  canSave: boolean
  saving: boolean
  resetting: boolean
  busy: boolean
  onSave: () => void
  onReset: () => void
}

function ActionFooter({
  settings,
  hasChanges,
  canSave,
  saving,
  resetting,
  busy,
  onSave,
  onReset,
}: Readonly<ActionFooterProps>) {
  const hasOverride = settings.source === 'team'

  return (
    <Card>
      <CardContent className="flex flex-wrap items-center justify-between gap-3 pt-4">
        <div className="text-muted-foreground space-y-1 text-sm">
          {hasChanges && <p>You have unsaved changes.</p>}
          {hasOverride && (
            <p>
              Reset would restore the instance defaults:{' '}
              {describeValues(settings.instance_defaults)}.
            </p>
          )}
        </div>
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
          <Button size="sm" disabled={!canSave} onClick={onSave}>
            {saving ? 'Saving…' : 'Save changes'}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export function SearchSettings() {
  const { currentTeam } = useTeam()
  const { can } = usePermissions()
  const canEdit = can('team.settings.update')

  const [settings, setSettings] = useState<TeamSearchSettings | null>(null)
  const [form, setForm] = useState<RankingForm | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [resetting, setResetting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [successMessage, setSuccessMessage] = useState<string | null>(null)

  const teamId = currentTeam?.id

  const load = useCallback(async () => {
    if (!teamId) return
    try {
      setLoading(true)
      setError(null)
      const response = await searchSettingsService.getSearchSettings(teamId)
      setSettings(response)
      setForm(toForm(effectiveValues(response)))
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to load search settings'
      )
    } finally {
      setLoading(false)
    }
  }, [teamId])

  useEffect(() => {
    void load()
  }, [load])

  const selectPreset = (presetId: RankingPresetId) => {
    const preset = RANKING_PRESETS.find(p => p.id === presetId)
    if (!preset) return
    setSuccessMessage(null)
    setForm(prev => {
      if (!prev) return prev
      // "Relevance only" only flips the switch — the weights are irrelevant
      // while recency ranking is off, so the team's tuning survives a round
      // trip through it.
      if (!preset.values) return { ...prev, recency_ranking_enabled: false }
      return toForm(preset.values)
    })
  }

  const handleNumericChange = (key: NumericField, value: string) => {
    setSuccessMessage(null)
    setForm(prev => (prev ? { ...prev, [key]: value } : prev))
  }

  const handleSave = async () => {
    if (!teamId || !form) return
    try {
      setSaving(true)
      setError(null)
      setSuccessMessage(null)
      const updated = await searchSettingsService.updateSearchSettings(
        teamId,
        toValues(form)
      )
      setSettings(updated)
      setForm(toForm(effectiveValues(updated)))
      setSuccessMessage('Search ranking settings saved.')
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to save search settings'
      )
    } finally {
      setSaving(false)
    }
  }

  const handleReset = async () => {
    if (!teamId) return
    try {
      setResetting(true)
      setError(null)
      setSuccessMessage(null)
      await searchSettingsService.resetSearchSettings(teamId)
      await load()
      setSuccessMessage('Reset to the instance defaults.')
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to reset search settings'
      )
    } finally {
      setResetting(false)
    }
  }

  const header = (
    <PageHeader
      title="Search Settings"
      description="Choose how search results are ordered for everyone in this team."
    />
  )

  if (loading) {
    return (
      <div className="space-y-6">
        {header}
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </div>
    )
  }

  const errorAlert = error && (
    <Alert variant="destructive">
      <AlertTitle>Error</AlertTitle>
      <AlertDescription>{error}</AlertDescription>
    </Alert>
  )

  if (!settings || !form) {
    return (
      <div className="space-y-6">
        {header}
        {errorAlert}
      </div>
    )
  }

  const currentValues = toValues(form)
  const validationError = validate(form)
  const hasChanges = !sameValues(currentValues, effectiveValues(settings))
  const busy = saving || resetting

  return (
    <div className="space-y-6">
      {header}
      {errorAlert}

      {successMessage && (
        <Alert>
          <AlertTitle>Saved</AlertTitle>
          <AlertDescription>{successMessage}</AlertDescription>
        </Alert>
      )}

      <ProfileCard
        selectedPreset={detectPreset(currentValues)}
        canEdit={canEdit}
        busy={busy}
        isInherited={settings.source === 'instance'}
        onSelect={selectPreset}
      />

      <AdvancedCard
        form={form}
        candidateCap={settings.rank_candidate_cap}
        canEdit={canEdit}
        busy={busy}
        validationError={validationError}
        onNumericChange={handleNumericChange}
      />

      {canEdit && (
        <ActionFooter
          settings={settings}
          hasChanges={hasChanges}
          canSave={!busy && hasChanges && validationError === null}
          saving={saving}
          resetting={resetting}
          busy={busy}
          onSave={() => {
            void handleSave()
          }}
          onReset={() => {
            void handleReset()
          }}
        />
      )}
    </div>
  )
}
