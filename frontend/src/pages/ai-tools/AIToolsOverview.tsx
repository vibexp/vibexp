import { Activity, Bot, Settings, TrendingUp } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { ClaudeCodeIcon, CursorIDEIcon } from '@/components/icons'
import { PageHeader } from '@/components/PageHeader'
import { StatusBadge } from '@/components/StatusBadge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { aiToolsService } from '@/services/aiToolsService'

interface Tool {
  name: 'Claude Code' | 'Cursor IDE'
  description: string
  sessions: number
  href: string
}

export function AIToolsOverview() {
  const navigate = useNavigate()
  const [claudeSessions, setClaudeSessions] = useState(0)
  const [cursorSessions, setCursorSessions] = useState(0)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const fetchCounts = async () => {
      try {
        const [claudeRes, cursorRes] = await Promise.all([
          aiToolsService.getClaudeCodeSessionCounts('7d'),
          aiToolsService.getCursorIDESessionCounts('7d'),
        ])
        setClaudeSessions(claudeRes.total_sessions)
        setCursorSessions(cursorRes.total_sessions)
      } catch (error) {
        console.error('Failed to fetch session counts:', error)
      } finally {
        setLoading(false)
      }
    }
    void fetchCounts()
  }, [])

  const tools: Tool[] = [
    {
      name: 'Claude Code',
      description:
        'AI-powered code assistant with advanced context awareness and debugging capabilities.',
      sessions: claudeSessions,
      href: '/ai-tools/claude-code/overview',
    },
    {
      name: 'Cursor IDE',
      description:
        'AI-powered IDE with intelligent code completion and editing.',
      sessions: cursorSessions,
      href: '/ai-tools/cursor-ide/overview',
    },
  ]

  const totalSessions = tools.reduce((sum, t) => sum + t.sessions, 0)

  return (
    <div className="space-y-6">
      <PageHeader
        title="AI Tools"
        description="Configure and monitor your AI development tools."
      />

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <StatCard
          label="Total tools"
          value={tools.length}
          icon={Settings}
          loading={false}
        />
        <StatCard
          label="Active sessions (7d)"
          value={totalSessions}
          icon={Activity}
          loading={loading}
        />
        <StatCard
          label="Connected tools"
          value={tools.length}
          icon={TrendingUp}
          loading={false}
        />
      </div>

      <section className="space-y-3">
        <h2 className="text-lg font-semibold">Available AI tools</h2>
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          {tools.map(tool => (
            <Card key={tool.name}>
              <CardHeader>
                <div className="flex items-center gap-3">
                  {tool.name === 'Claude Code' ? (
                    <ClaudeCodeIcon
                      className="text-foreground"
                      width={112}
                      height={24}
                    />
                  ) : (
                    <CursorIDEIcon
                      className="text-foreground"
                      width={90}
                      height={22}
                    />
                  )}
                </div>
                <CardTitle className="sr-only">{tool.name}</CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                <p className="text-muted-foreground text-sm">
                  {tool.description}
                </p>
                <div className="space-y-2 text-sm">
                  <Row label="Status">
                    <StatusBadge tone="success">Connected</StatusBadge>
                  </Row>
                  <Row label="Sessions (7d)">
                    {loading ? (
                      <Skeleton className="h-4 w-10" />
                    ) : (
                      <span className="font-medium">{tool.sessions}</span>
                    )}
                  </Row>
                </div>
                <Button
                  className="w-full"
                  onClick={() => {
                    void navigate(tool.href)
                  }}
                >
                  Manage
                </Button>
              </CardContent>
            </Card>
          ))}
        </div>
      </section>
    </div>
  )
}

function StatCard({
  label,
  value,
  icon: Icon,
  loading,
}: Readonly<{
  label: string
  value: number
  icon: typeof Bot
  loading: boolean
}>) {
  return (
    <Card>
      <CardContent className="flex items-center justify-between p-6">
        <div className="space-y-1">
          <p className="text-muted-foreground text-sm">{label}</p>
          {loading ? (
            <Skeleton className="h-8 w-12" />
          ) : (
            <p className="text-2xl font-semibold">{value}</p>
          )}
        </div>
        <div className="bg-muted flex size-12 items-center justify-center rounded-lg">
          <Icon className="size-5" />
        </div>
      </CardContent>
    </Card>
  )
}

function Row({
  label,
  children,
}: Readonly<{
  label: string
  children: React.ReactNode
}>) {
  return (
    <div className="flex items-center justify-between">
      <span className="text-muted-foreground">{label}</span>
      {children}
    </div>
  )
}
