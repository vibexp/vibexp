import { AlertCircle, ArrowLeft, Save } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'

import { PageHeader } from '@/components/PageHeader'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { useTeam } from '@/contexts/TeamContext'
import { toast } from '@/lib/toast'
import type {
  Agent,
  AgentCard,
  CreateAgentRequest,
  UpdateAgentRequest,
} from '@/services/agentService'
import { agentService } from '@/services/agentService'
import { getErrorMessage } from '@/utils/errorHandling'

import { AgentCredentialsEditor } from './editor/AgentCredentialsEditor'
import { AgentPreview } from './editor/AgentPreview'

interface AgentFormData {
  baseUrl: string
  status: 'active' | 'paused'
}

interface AgentPreviewState {
  loading: boolean
  data: AgentCard | null
  error: string | null
}

function buildAgentCardUrl(baseUrl: string): string {
  const cleanBase = baseUrl.trim().replace(/\/$/, '')
  return `${cleanBase}/.well-known/agent-card.json`
}

export function AgentEditor() {
  const navigate = useNavigate()
  const { id } = useParams<{ id: string }>()
  const { currentTeam } = useTeam()

  const [loading, setLoading] = useState(!!id)
  const [saving, setSaving] = useState(false)
  const [agent, setAgent] = useState<Agent | null>(null)
  const [formData, setFormData] = useState<AgentFormData>({
    baseUrl: '',
    status: 'active',
  })
  const [errors, setErrors] = useState<Partial<AgentFormData>>({})
  const [preview, setPreview] = useState<AgentPreviewState>({
    loading: false,
    data: null,
    error: null,
  })

  const fetchDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const isEditing = !!id
  const pageTitle = isEditing ? 'Edit agent' : 'Add agent'

  const loadAgent = useCallback(
    async (agentId: string, teamId: string) => {
      try {
        setLoading(true)
        const agentData = await agentService.getAgent(teamId, agentId)
        setAgent(agentData)

        let baseUrl = agentData.card_url ?? ''
        if (baseUrl.endsWith('/.well-known/agent-card.json')) {
          baseUrl = baseUrl.replace('/.well-known/agent-card.json', '')
        }

        setFormData({
          baseUrl,
          status: agentData.status === 'error' ? 'paused' : agentData.status,
        })
      } catch (err) {
        toast.error(getErrorMessage(err, 'Failed to load agent'))
        void navigate('/agents')
      } finally {
        setLoading(false)
      }
    },
    [navigate]
  )

  useEffect(() => {
    if (id && currentTeam) {
      void loadAgent(id, currentTeam.id)
    }
  }, [id, currentTeam, loadAgent])

  const fetchAgentPreview = useCallback(
    async (baseUrl: string) => {
      if (!baseUrl.trim()) {
        setPreview({ loading: false, data: null, error: null })
        return
      }
      try {
        new URL(baseUrl)
      } catch {
        setPreview({ loading: false, data: null, error: 'Invalid URL format' })
        return
      }
      if (!currentTeam) {
        setPreview({ loading: false, data: null, error: 'No team selected' })
        return
      }

      setPreview(prev => ({ ...prev, loading: true, error: null }))
      try {
        const cardUrl = buildAgentCardUrl(baseUrl)
        const data = await agentService.previewAgentCard(
          currentTeam.id,
          cardUrl
        )
        setPreview({ loading: false, data, error: null })
      } catch (error) {
        setPreview({
          loading: false,
          data: null,
          error: getErrorMessage(error, 'Failed to fetch agent card'),
        })
      }
    },
    [currentTeam]
  )

  useEffect(() => {
    if (fetchDebounceRef.current) {
      clearTimeout(fetchDebounceRef.current)
    }
    fetchDebounceRef.current = setTimeout(() => {
      void fetchAgentPreview(formData.baseUrl)
    }, 800)
    return () => {
      if (fetchDebounceRef.current) {
        clearTimeout(fetchDebounceRef.current)
      }
    }
  }, [formData.baseUrl, fetchAgentPreview])

  const validate = (): boolean => {
    const newErrors: Partial<AgentFormData> = {}
    if (!formData.baseUrl.trim()) {
      newErrors.baseUrl = 'Agent base URL is required'
    } else {
      try {
        new URL(formData.baseUrl)
      } catch {
        newErrors.baseUrl = 'Please enter a valid URL'
      }
    }
    setErrors(newErrors)
    if (Object.keys(newErrors).length > 0) return false
    if (!preview.data) {
      toast.error('Please wait for agent preview to load successfully')
      return false
    }
    return true
  }

  const handleSave = async () => {
    if (!validate()) return
    if (!currentTeam) {
      toast.error('No team selected')
      return
    }

    try {
      setSaving(true)
      const cardUrl = buildAgentCardUrl(formData.baseUrl)
      if (isEditing && agent) {
        const updateData: UpdateAgentRequest = {
          card_url: cardUrl,
          status: formData.status,
        }
        await agentService.updateAgent(currentTeam.id, agent.id, updateData)
        toast.success('Agent updated successfully')
      } else {
        const createData: CreateAgentRequest = {
          card_url: cardUrl,
          status: formData.status,
        }
        await agentService.createAgent(currentTeam.id, createData)
        toast.success(`Agent "${preview.data?.name ?? 'Agent'}" created`)
      }
      void navigate('/agents')
    } catch (error) {
      toast.error(getErrorMessage(error, 'Failed to save agent'))
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <div className="space-y-6">
        <PageHeader title={pageTitle} description="Loading…" />
        <Skeleton className="h-64 w-full" />
      </div>
    )
  }

  const securitySchemes = preview.data?.securitySchemes
  const showCredentialsEditor =
    isEditing &&
    !!agent &&
    !!currentTeam &&
    !!securitySchemes &&
    Object.keys(securitySchemes).length > 0

  const submitLabel = isEditing ? 'Update agent' : 'Create agent'

  return (
    <div className="space-y-6">
      <Button
        variant="ghost"
        size="sm"
        onClick={() => {
          void navigate('/agents')
        }}
      >
        <ArrowLeft className="mr-2 size-4" />
        Back to agents
      </Button>

      <PageHeader
        title={pageTitle}
        description={
          isEditing
            ? `Editing: ${agent?.name ?? ''}`
            : 'Add your A2A-compliant agent to interact directly from VibeXP.'
        }
        actions={
          <Button
            onClick={() => {
              void handleSave()
            }}
            disabled={saving || !preview.data || preview.loading}
          >
            <Save className="mr-2 size-4" />
            {saving ? 'Saving…' : submitLabel}
          </Button>
        }
      />

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        <div className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Agent configuration</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-1.5">
                <Label htmlFor="baseUrl">
                  Agent base URL <span className="text-destructive">*</span>
                </Label>
                <Input
                  id="baseUrl"
                  type="url"
                  value={formData.baseUrl}
                  onChange={e => {
                    setFormData(prev => ({
                      ...prev,
                      baseUrl: e.target.value,
                    }))
                  }}
                  placeholder="https://your-agent-domain.com"
                  disabled={isEditing}
                  aria-invalid={!!errors.baseUrl}
                />
                {errors.baseUrl && (
                  <p className="text-destructive flex items-center gap-1 text-xs">
                    <AlertCircle className="size-3" />
                    {errors.baseUrl}
                  </p>
                )}
                <p className="text-muted-foreground text-xs">
                  We&apos;ll fetch the agent card from:
                </p>
                <code className="bg-muted block rounded p-2 text-xs break-all">
                  {formData.baseUrl || 'https://domain.com'}
                  /.well-known/agent-card.json
                </code>
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="status">Initial status</Label>
                <Select
                  value={formData.status}
                  onValueChange={v => {
                    setFormData(prev => ({
                      ...prev,
                      status: v as 'active' | 'paused',
                    }))
                  }}
                >
                  <SelectTrigger id="status">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="active">Active</SelectItem>
                    <SelectItem value="paused">Paused</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </CardContent>
          </Card>

          {showCredentialsEditor && (
            <AgentCredentialsEditor
              agentId={agent.id}
              teamId={currentTeam.id}
              securitySchemes={securitySchemes}
              hasCredentials={agent.has_credentials}
            />
          )}
        </div>

        <div className="lg:col-span-2">
          <div className="mb-3">
            <h3 className="text-lg font-semibold">Agent card preview</h3>
          </div>
          <AgentPreview
            loading={preview.loading}
            data={preview.data}
            error={preview.error}
            onRetry={() => {
              if (formData.baseUrl) {
                void fetchAgentPreview(formData.baseUrl)
              }
            }}
          />
        </div>
      </div>
    </div>
  )
}
