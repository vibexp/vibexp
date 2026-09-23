import { RotateCcw, Sparkles } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'

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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { usePermissions } from '@/hooks/usePermissions'
import {
  aiSummarySettingsService,
  type TeamAISummarySettings,
} from '@/services/aiSummarySettingsService'
import type { ModelProviderResponse } from '@/services/modelProviderService'
import type { Team } from '@/services/teamService'

import {
  AI_SUMMARY_STYLES,
  type AISummaryForm,
  type AISummaryStyle,
  clampTopN,
  describeValues,
  sameValues,
  toForm,
  toValues,
  validate,
} from './aiSummaryForm'

const selectClass =
  'border-input bg-background focus-visible:ring-ring h-9 w-full rounded-md border px-3 py-1 text-sm focus-visible:ring-1 focus-visible:outline-none'

// Native <select> rather than the Radix one: it needs no jsdom layout shims and
// stays safe if this card is ever rendered inside a Dialog.
const TEAM_DEFAULT = ''

interface FieldsProps {
  form: AISummaryForm
  providers: ModelProviderResponse[]
  maxTopN: number
  disabled: boolean
  onChange: (patch: Partial<AISummaryForm>) => void
}

function AiSummaryFields({
  form,
  providers,
  maxTopN,
  disabled,
  onChange,
}: Readonly<FieldsProps>) {
  return (
    <fieldset
      className="space-y-4"
      disabled={disabled}
      aria-label="AI Summary settings"
    >
      <div className="flex items-center justify-between gap-3 rounded-md border p-3">
        <div className="space-y-1">
          <Label htmlFor="ai-summary-enabled">Enable AI Summary</Label>
          <p className="text-muted-foreground text-xs">
            Summarize the top search results with this team&apos;s model.
          </p>
        </div>
        <Switch
          id="ai-summary-enabled"
          checked={form.enabled}
          disabled={disabled}
          onCheckedChange={checked => {
            onChange({ enabled: checked })
          }}
        />
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-1">
          <Label htmlFor="ai-summary-provider">Provider</Label>
          <select
            id="ai-summary-provider"
            className={selectClass}
            value={form.model_provider_id ?? TEAM_DEFAULT}
            onChange={e => {
              onChange({ model_provider_id: e.target.value || null })
            }}
          >
            <option value={TEAM_DEFAULT}>Team default</option>
            {providers.map(provider => (
              <option key={provider.id} value={provider.id}>
                {provider.name} — {provider.model}
              </option>
            ))}
          </select>
          <p className="text-muted-foreground text-xs">
            Each provider uses the one model it is configured with.
          </p>
        </div>

        <div className="space-y-1">
          <Label htmlFor="ai-summary-style">Style</Label>
          <select
            id="ai-summary-style"
            className={selectClass}
            value={form.style}
            onChange={e => {
              onChange({ style: e.target.value as AISummaryStyle })
            }}
          >
            {AI_SUMMARY_STYLES.map(style => (
              <option key={style.id} value={style.id}>
                {style.label}
              </option>
            ))}
          </select>
        </div>

        <div className="space-y-1">
          <Label htmlFor="ai-summary-top-n">Results to read</Label>
          <Input
            id="ai-summary-top-n"
            type="number"
            min={1}
            max={maxTopN}
            step={1}
            value={form.top_n}
            onChange={e => {
              onChange({ top_n: clampTopN(e.target.value, maxTopN) })
            }}
          />
          <p className="text-muted-foreground text-xs">
            How many top-ranked results the summary reads (1–{maxTopN}).
          </p>
        </div>

        <div className="space-y-1">
          <Label htmlFor="ai-summary-max-tokens">
            Response length (tokens)
          </Label>
          <Input
            id="ai-summary-max-tokens"
            type="number"
            min={1}
            step={1}
            value={form.max_output_tokens}
            onChange={e => {
              onChange({ max_output_tokens: e.target.value })
            }}
          />
          <p className="text-muted-foreground text-xs">
            Upper bound on how much the summary may write.
          </p>
        </div>
      </div>
    </fieldset>
  )
}

interface AiSummarySettingsProps {
  team: Team
  providers: ModelProviderResponse[]
  /** Bumped by the page after a provider is added, updated, copied or deleted. */
  reloadKey: number
}

/**
 * The team's AI Summary profile (#1077), shown on the Model Providers page
 * because a provider is what makes the feature work.
 *
 * Every read/write keys on the URL-resolved `team` prop, never the ambient
 * team (#584). Editing is gated on `team.settings.update` — the permission the
 * PUT/DELETE ops authorize — and never on `role`.
 */
export function AiSummarySettings({
  team,
  providers,
  reloadKey,
}: Readonly<AiSummarySettingsProps>) {
  const { handleError } = useErrorHandler()
  const { can } = usePermissions(team)
  const canEdit = can('team.settings.update')
  const teamId = team.id

  const [settings, setSettings] = useState<TeamAISummarySettings | null>(null)
  const [form, setForm] = useState<AISummaryForm | null>(null)
  const [saving, setSaving] = useState(false)
  const [resetting, setResetting] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [successMessage, setSuccessMessage] = useState<string | null>(null)
  const [loadFailed, setLoadFailed] = useState(false)
  // Guards against a slow response for an older reload overwriting a newer one.
  const loadSeq = useRef(0)

  const load = useCallback(async (): Promise<boolean> => {
    const seq = ++loadSeq.current
    try {
      const response =
        await aiSummarySettingsService.getAISummarySettings(teamId)
      if (seq !== loadSeq.current) return false
      setSettings(response)
      setForm(toForm(response.values))
      setLoadFailed(false)
      return true
    } catch (error) {
      if (seq === loadSeq.current) {
        setLoadFailed(true)
        handleError(error, 'Failed to load AI Summary settings')
      }
      return false
    }
  }, [handleError, teamId])

  useEffect(() => {
    void load()
  }, [load, reloadKey])

  if ((!settings || !form) && loadFailed) {
    return (
      <Card data-testid="ai-summary-load-failed">
        <CardContent className="text-muted-foreground p-6 text-sm">
          Couldn&apos;t load the AI Summary settings. Reload the page to try
          again.
        </CardContent>
      </Card>
    )
  }

  if (!settings || !form) {
    return (
      <Card data-testid="ai-summary-loading">
        <CardContent className="space-y-3 p-6">
          <Skeleton className="h-6 w-40" />
          <Skeleton className="h-10 w-full" />
        </CardContent>
      </Card>
    )
  }

  const providerName = (id: string | null) =>
    providers.find(p => p.id === id)?.name ?? 'team default provider'

  const handleChange = (patch: Partial<AISummaryForm>) => {
    setSuccessMessage(null)
    setForm(prev => (prev ? { ...prev, ...patch } : prev))
  }

  const handleSave = async () => {
    try {
      setSaving(true)
      setSaveError(null)
      setSuccessMessage(null)
      const updated = await aiSummarySettingsService.updateAISummarySettings(
        teamId,
        toValues(form)
      )
      setSettings(updated)
      setForm(toForm(updated.values))
      setSuccessMessage('AI Summary settings saved.')
    } catch (error) {
      // The form keeps the user's edits so they can correct and retry.
      setSaveError(
        error instanceof Error
          ? error.message
          : 'Failed to save AI Summary settings'
      )
    } finally {
      setSaving(false)
    }
  }

  const handleReset = async () => {
    try {
      setResetting(true)
      setSaveError(null)
      setSuccessMessage(null)
      await aiSummarySettingsService.resetAISummarySettings(teamId)
      if (await load()) {
        setSuccessMessage('Reset to the instance defaults.')
      }
    } catch (error) {
      setSaveError(
        error instanceof Error
          ? error.message
          : 'Failed to reset AI Summary settings'
      )
    } finally {
      setResetting(false)
    }
  }

  const isTeamOwned = settings.source === 'team'
  const busy = saving || resetting
  const validationError = validate(form, settings.max_top_n)
  const hasChanges = !sameValues(toValues(form), settings.values)

  return (
    <Card data-testid="ai-summary-settings">
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <CardTitle className="flex items-center gap-2">
            <Sparkles className="size-4" />
            AI Summary
          </CardTitle>
          <Badge variant="secondary">
            {isTeamOwned
              ? 'Customized for this team'
              : 'Using instance defaults'}
          </Badge>
        </div>
        <CardDescription>
          Let search summarize its top results with one of this team&apos;s
          model providers.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {!settings.available ? (
          <p
            className="text-muted-foreground text-sm"
            data-testid="ai-summary-unavailable"
          >
            AI Summary needs a model provider. Add one above, then come back
            here to turn it on.
          </p>
        ) : (
          <>
            {!canEdit && (
              <p className="text-muted-foreground text-sm">
                Only team owners and admins can change these settings.
              </p>
            )}
            <AiSummaryFields
              form={form}
              providers={providers}
              maxTopN={settings.max_top_n}
              disabled={!canEdit || busy}
              onChange={handleChange}
            />
            {validationError && (
              <Alert variant="destructive">
                <AlertTitle>Invalid values</AlertTitle>
                <AlertDescription>{validationError}</AlertDescription>
              </Alert>
            )}
            {saveError && (
              <Alert variant="destructive">
                <AlertTitle>Error</AlertTitle>
                <AlertDescription>{saveError}</AlertDescription>
              </Alert>
            )}
            {successMessage && (
              <Alert>
                <AlertTitle>Saved</AlertTitle>
                <AlertDescription>{successMessage}</AlertDescription>
              </Alert>
            )}
            {canEdit && (
              <div className="flex flex-wrap items-center justify-between gap-3 border-t pt-4">
                <div className="text-muted-foreground space-y-1 text-sm">
                  {hasChanges && <p>You have unsaved changes.</p>}
                  {isTeamOwned && (
                    <p>
                      Reset would restore the instance defaults:{' '}
                      {describeValues(settings.instance_defaults, providerName)}
                      .
                    </p>
                  )}
                </div>
                <div className="flex gap-2">
                  {isTeamOwned && (
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={busy}
                      onClick={() => {
                        void handleReset()
                      }}
                    >
                      <RotateCcw className="mr-2 size-4" />
                      {resetting ? 'Resetting…' : 'Reset to defaults'}
                    </Button>
                  )}
                  <Button
                    size="sm"
                    disabled={busy || !hasChanges || validationError !== null}
                    onClick={() => {
                      void handleSave()
                    }}
                  >
                    {saving ? 'Saving…' : 'Save changes'}
                  </Button>
                </div>
              </div>
            )}
          </>
        )}
      </CardContent>
    </Card>
  )
}
