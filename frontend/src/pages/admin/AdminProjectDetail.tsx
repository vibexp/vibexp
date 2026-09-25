import { useEffect, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router'

import { PageHeader } from '@/components/PageHeader'
import { Card, CardContent } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatDate } from '@/lib/time'
import { AdminDetailScaffold } from '@/pages/admin/AdminDetailScaffold'
import { resourceCountLabel } from '@/pages/admin/projects/detail/projectChartData'
import { ProjectConfigTab } from '@/pages/admin/projects/detail/ProjectConfigTab'
import { ProjectOverviewTab } from '@/pages/admin/projects/detail/ProjectOverviewTab'
import type { AdminProjectDetail as AdminProjectDetailType } from '@/services/adminService'
import { adminService } from '@/services/adminService'
import { getErrorMessage } from '@/utils/errorHandling'

/** A metadata cell; renders an em dash for the empty-string columns the API returns. */
function Field({
  label,
  children,
}: Readonly<{ label: string; children: React.ReactNode }>) {
  return (
    <div>
      <p className="text-muted-foreground text-xs">{label}</p>
      <div className="text-sm break-words">{children}</div>
    </div>
  )
}

/**
 * The resource-count panel is driven by the response, not by a hardcoded type
 * list.
 *
 * The API reports only the five **project-scoped** tables plus their total
 * (#453, feed items and the total since #1143) — agents and feeds have no
 * `project_id`, so they are absent rather than reported as `0`,
 * which would read as "this project has no agents" when the truth is "agents do
 * not belong to projects". A hardcoded list here would reintroduce exactly that
 * lie, and would silently drop a type the API adds later.
 */
function ResourceCounts({
  counts,
}: Readonly<{ counts: AdminProjectDetailType['resource_counts'] }>) {
  const entries = Object.entries(counts)
  return (
    <div className="space-y-2">
      <h2 className="text-sm font-semibold">Resources</h2>
      <Card>
        <CardContent className="grid grid-cols-2 gap-4 py-4 sm:grid-cols-4">
          {entries.map(([key, value]) => (
            <div key={key}>
              <p className="text-muted-foreground text-xs">
                {/* Fall back to the raw key so a type added to the API shows up
                    labelled-ish rather than not at all. */}
                {resourceCountLabel(key) ?? key}
              </p>
              <p className="text-2xl font-semibold tabular-nums">
                {String(value)}
              </p>
            </div>
          ))}
        </CardContent>
      </Card>
    </div>
  )
}

const TABS = ['overview', 'configuration'] as const
type ProjectDetailTab = (typeof TABS)[number]

function isTab(value: string | null): value is ProjectDetailTab {
  return TABS.includes(value as ProjectDetailTab)
}

/**
 * Instance project detail — metadata, team, creator and resource counts (#461)
 * above two read-only tabs (#1146). The active tab lives in `?tab=` so it
 * survives a reload and can be linked to; Radix unmounts inactive tabs, so
 * the configuration is fetched only when its tab is opened.
 */
export function AdminProjectDetail() {
  const { id } = useParams<{ id: string }>()
  const [searchParams, setSearchParams] = useSearchParams()
  const requestedTab = searchParams.get('tab')
  const tab: ProjectDetailTab = isTab(requestedTab) ? requestedTab : 'overview'
  const setTab = (value: string) => {
    setSearchParams(
      prev => {
        const next = new URLSearchParams(prev)
        next.set('tab', value)
        return next
      },
      { replace: true }
    )
  }
  const [project, setProject] = useState<AdminProjectDetailType | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!id) return
    let cancelled = false
    setLoading(true)
    setError(null)
    adminService
      .getProject(id)
      .then(result => {
        if (!cancelled) setProject(result)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(getErrorMessage(err, 'Failed to load project'))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [id])

  return (
    <AdminDetailScaffold
      backTo="/admin/projects"
      backLabel="Back to projects"
      loading={loading}
      error={error}
      errorTitle="Failed to load project"
    >
      {project && (
        <>
          <PageHeader title={project.name} description={project.slug} />
          <Card>
            <CardContent className="grid grid-cols-1 gap-4 py-4 sm:grid-cols-3">
              <Field label="Team">
                {/* Both admin detail pages already exist, so the team and the
                    creator are reachable in one click from here. */}
                <Link
                  to={`/admin/teams/${project.team.id}`}
                  className="hover:underline"
                >
                  {project.team.name}
                </Link>
              </Field>
              <Field label="Owner">
                <Link
                  to={`/admin/users/${project.owner.id}`}
                  className="hover:underline"
                >
                  {project.owner.email}
                </Link>
              </Field>
              <Field label="Created">{formatDate(project.created_at)}</Field>
              <Field label="Description">{project.description || '—'}</Field>
              <Field label="Git URL">
                {project.git_url ? (
                  <span className="font-mono text-xs">{project.git_url}</span>
                ) : (
                  '—'
                )}
              </Field>
              <Field label="Homepage">
                {project.homepage ? (
                  <span className="font-mono text-xs">{project.homepage}</span>
                ) : (
                  '—'
                )}
              </Field>
              <Field label="Updated">{formatDate(project.updated_at)}</Field>
            </CardContent>
          </Card>

          <ResourceCounts counts={project.resource_counts} />

          <Tabs value={tab} onValueChange={setTab} className="space-y-6">
            <TabsList>
              <TabsTrigger value="overview">Overview</TabsTrigger>
              <TabsTrigger value="configuration">Configuration</TabsTrigger>
            </TabsList>
            <TabsContent value="overview">
              <ProjectOverviewTab
                projectId={project.id}
                resourceCounts={project.resource_counts}
              />
            </TabsContent>
            <TabsContent value="configuration">
              <ProjectConfigTab
                projectId={project.id}
                projectName={project.name}
                teamId={project.team.id}
              />
            </TabsContent>
          </Tabs>
        </>
      )}
    </AdminDetailScaffold>
  )
}
