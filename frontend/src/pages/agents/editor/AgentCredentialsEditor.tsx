import { Eye, EyeOff, Save, Shield } from 'lucide-react'
import { useState } from 'react'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/lib/toast'
import { agentService } from '@/services/agentService'
import { getErrorMessage } from '@/utils/errorHandling'

interface Credential {
  name: string
  type: string
  value: string
  showValue: boolean
}

interface AgentCredentialsEditorProps {
  agentId: string
  teamId: string
  securitySchemes?: Record<string, unknown>
  hasCredentials?: string[]
}

const MASKED = '••••••••'

export function AgentCredentialsEditor({
  agentId,
  teamId,
  securitySchemes,
  hasCredentials,
}: AgentCredentialsEditorProps) {
  const [credentials, setCredentials] = useState<Credential[]>(() => {
    if (!securitySchemes || Object.keys(securitySchemes).length === 0) {
      return []
    }
    return Object.entries(securitySchemes).map(([name, scheme]) => {
      const schemeData = scheme as Record<string, unknown>
      const isSet = hasCredentials?.includes(name) ?? false
      return {
        name,
        type: (schemeData.type as string) || 'unknown',
        value: isSet ? MASKED : '',
        showValue: false,
      }
    })
  })
  const [updating, setUpdating] = useState(false)

  const handleCredentialChange = (index: number, value: string) => {
    setCredentials(prev =>
      prev.map((cred, i) => (i === index ? { ...cred, value } : cred))
    )
  }

  const toggleShowValue = (index: number) => {
    setCredentials(prev =>
      prev.map((cred, i) =>
        i === index ? { ...cred, showValue: !cred.showValue } : cred
      )
    )
  }

  const handleUpdateCredentials = async () => {
    const filled = credentials.filter(
      c => c.value.trim() !== '' && c.value !== MASKED
    )

    if (filled.length === 0) {
      toast.error('Please provide at least one credential value')
      return
    }

    try {
      setUpdating(true)
      const payload: Record<string, { type: string; value: string }> = {}
      filled.forEach(cred => {
        payload[cred.name] = {
          type: cred.type === 'oauth2' ? 'oauth2' : 'apiKey',
          value: cred.value,
        }
      })
      await agentService.updateAgentCredentials(teamId, agentId, payload)
      toast.success('Agent credentials updated successfully')
      setCredentials(prev =>
        prev.map(c => ({ ...c, value: '', showValue: false }))
      )
    } catch (error) {
      toast.error(getErrorMessage(error, 'Failed to update credentials'))
    } finally {
      setUpdating(false)
    }
  }

  if (!securitySchemes || Object.keys(securitySchemes).length === 0) {
    return null
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Shield className="size-4" />
          Security credentials
        </CardTitle>
        <p className="text-muted-foreground text-sm">
          Update authentication credentials for this agent.
        </p>
      </CardHeader>
      <CardContent className="space-y-4">
        {credentials.map((credential, index) => (
          <div key={credential.name} className="space-y-1.5">
            <Label htmlFor={`credential-${credential.name}`}>
              {credential.name}
              <span className="text-muted-foreground ml-2 text-xs font-normal">
                ({credential.type})
              </span>
            </Label>
            <div className="relative">
              <Input
                id={`credential-${credential.name}`}
                type={credential.showValue ? 'text' : 'password'}
                value={credential.value}
                onChange={e => {
                  handleCredentialChange(index, e.target.value)
                }}
                placeholder={`Enter ${credential.name}`}
                className="pr-10"
              />
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="absolute right-1 top-1/2 size-8 -translate-y-1/2"
                onClick={() => {
                  toggleShowValue(index)
                }}
                aria-label={credential.showValue ? 'Hide value' : 'Show value'}
              >
                {credential.showValue ? (
                  <EyeOff className="size-4" />
                ) : (
                  <Eye className="size-4" />
                )}
              </Button>
            </div>
          </div>
        ))}

        <Button
          className="w-full"
          onClick={() => {
            void handleUpdateCredentials()
          }}
          disabled={updating}
        >
          <Save className="mr-2 size-4" />
          {updating ? 'Updating…' : 'Update credentials'}
        </Button>
      </CardContent>
    </Card>
  )
}
