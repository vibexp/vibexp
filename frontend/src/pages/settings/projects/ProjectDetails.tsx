import {
  AlertCircle,
  ArrowLeft,
  ArrowRightLeft,
  Calendar,
  CheckCircle2,
  FileText,
  FolderKanban,
  GitBranch,
  Globe,
  Pencil,
  Rss,
  Sparkles,
  Trash2,
} from 'lucide-react'
import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'

import { AccessActivityPanel } from '@/components/access-activity/AccessActivityPanel'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { PageHeader } from '@/components/PageHeader'
import { ResourceCreationChart } from '@/components/ResourceCreationChart'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { usePermissions } from '@/hooks/usePermissions'
import type { Project, ProjectStatsResponse } from '@/services/projectService'
import { projectService } from '@/services/projectService'
import { getErrorMessage } from '@/utils/errorHandling'

const formatDate = (dateString: string) =>
  new Date(dateString).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })

function ProjectDetailsSkeleton() {
  return (
    <div className="space-y-6">
      <Skeleton className="h-8 w-32" />
      <Skeleton className="h-12 w-2/3" />
      <Skeleton className="h-40 w-full" />
      <div className="grid grid-cols-2 gap-4 md:grid-cols-5">
        {['s1', 's2', 's3', 's4', 's5'].map(key => (
          <Skeleton key={key} className="h-24 w-full" />
        ))}
      </div>
    </div>
  )
}

interface StatCardProps {
  label: string
  count: number
  icon: React.ElementType
}

function StatCard({ label, count, icon: Icon }: Readonly<StatCardProps>) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-muted-foreground flex items-center gap-2 text-sm font-medium">
          <Icon className="size-4" />
          {label}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-2xl font-semibold">{count}</p>
      </CardContent>
    </Card>
  )
}

export function ProjectDetails() {
  const { slug } = useParams<{ slug: string }>()
  const navigate = useNavigate()
  const { currentTeam, isLoading: isLoadingTeam } = useTeam()
  const { can } = usePermissions()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()

  const [project, setProject] = useState<Project | null>(null)
  const [stats, setStats] = useState<ProjectStatsResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [projectToDelete, setProjectToDelete] = useState<Project | null>(null)
  const [deleting, setDeleting] = useState(false)

  useEffect(() => {
    const load = async () => {
      if (isLoadingTeam) return
      if (!slug) {
        setError('Missing required context')
        setLoading(false)
        return
      }
      if (!currentTeam) {
        setError('No team available. Please select or create a team first.')
        setLoading(false)
        return
      }
      try {
        setLoading(true)
        setError(null)
        const [projectData, statsData] = await Promise.all([
          projectService.getProject(currentTeam.id, slug),
          projectService.getProjectStats(currentTeam.id, slug),
        ])
        setProject(projectData)
        setStats(statsData)
      } catch (err) {
        setError(getErrorMessage(err, 'Failed to load project'))
        handleError(err, 'Failed to load project')
      } finally {
        setLoading(false)
      }
    }
    void load()
  }, [slug, currentTeam, isLoadingTeam, handleError])

  const handleDelete = async () => {
    if (!projectToDelete || !currentTeam) return
    try {
      setDeleting(true)
      await projectService.deleteProject(currentTeam.id, projectToDelete.slug)
      showSuccess('Project deleted successfully', 'Success')
      void navigate('/settings/projects')
    } catch (err) {
      handleError(err, 'Failed to delete project')
    } finally {
      setDeleting(false)
      setProjectToDelete(null)
    }
  }

  if (isLoadingTeam || loading) {
    return <ProjectDetailsSkeleton />
  }

  if (error || !project) {
    return (
      <div className="space-y-4">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => {
            void navigate('/settings/projects')
          }}
        >
          <ArrowLeft className="mr-2 size-4" />
          Back to Projects
        </Button>
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Could not load project</AlertTitle>
          <AlertDescription>
            {error ?? 'The project could not be found.'}
          </AlertDescription>
        </Alert>
      </div>
    )
  }

  const encodedSlug = encodeURIComponent(project.slug)

  return (
    <div className="space-y-6">
      <Button
        variant="ghost"
        size="sm"
        onClick={() => {
          void navigate('/settings/projects')
        }}
      >
        <ArrowLeft className="mr-2 size-4" />
        Back to Projects
      </Button>

      <PageHeader
        title={project.name}
        description="Project details and resource overview"
        actions={
          <>
            {can('project.update') && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  void navigate(`/settings/projects/${encodedSlug}/migrate`)
                }}
              >
                <ArrowRightLeft className="mr-2 size-4" />
                Migrate Resources
              </Button>
            )}
            {can('project.update') && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  void navigate(`/settings/projects/edit/${encodedSlug}`)
                }}
              >
                <Pencil className="mr-2 size-4" />
                Edit
              </Button>
            )}
            {can('project.delete') && (
              <Button
                variant="destructive"
                size="sm"
                onClick={() => {
                  setProjectToDelete(project)
                }}
              >
                <Trash2 className="mr-2 size-4" />
                Delete
              </Button>
            )}
          </>
        }
      />

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <FolderKanban className="size-4" />
            Metadata
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {project.description && (
            <div>
              <p className="text-muted-foreground mb-1 text-xs font-medium uppercase tracking-wide">
                Description
              </p>
              <p className="text-sm">{project.description}</p>
            </div>
          )}

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <p className="text-muted-foreground mb-1 text-xs font-medium uppercase tracking-wide">
                Slug
              </p>
              <code className="bg-muted rounded px-2 py-1 font-mono text-sm">
                {project.slug}
              </code>
            </div>

            <div>
              <p className="text-muted-foreground mb-1 text-xs font-medium uppercase tracking-wide">
                GitHub Connected
              </p>
              {project.github_connected ? (
                <span className="text-success inline-flex items-center gap-1 text-sm">
                  <CheckCircle2 className="size-4" />
                  Connected
                </span>
              ) : (
                <span className="text-muted-foreground text-sm">
                  Not connected
                </span>
              )}
            </div>

            {project.git_url && (
              <div>
                <p className="text-muted-foreground mb-1 text-xs font-medium uppercase tracking-wide">
                  Git URL
                </p>
                <a
                  href={project.git_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-1 text-sm underline-offset-2 hover:underline"
                >
                  <GitBranch className="size-4 shrink-0" />
                  {project.git_url}
                </a>
              </div>
            )}

            {project.homepage && (
              <div>
                <p className="text-muted-foreground mb-1 text-xs font-medium uppercase tracking-wide">
                  Homepage
                </p>
                <a
                  href={project.homepage}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-1 text-sm underline-offset-2 hover:underline"
                >
                  <Globe className="size-4 shrink-0" />
                  {project.homepage}
                </a>
              </div>
            )}

            <div>
              <p className="text-muted-foreground mb-1 text-xs font-medium uppercase tracking-wide">
                Created
              </p>
              <span className="inline-flex items-center gap-1 text-sm">
                <Calendar className="size-4 shrink-0" />
                {formatDate(project.created_at)}
              </span>
            </div>

            <div>
              <p className="text-muted-foreground mb-1 text-xs font-medium uppercase tracking-wide">
                Last Updated
              </p>
              <span className="inline-flex items-center gap-1 text-sm">
                <Calendar className="size-4 shrink-0" />
                {formatDate(project.updated_at)}
              </span>
            </div>
          </div>
        </CardContent>
      </Card>

      {stats && (
        <section className="space-y-3">
          <h2 className="text-lg font-semibold">Resources (all time)</h2>
          <div className="grid grid-cols-2 gap-4 md:grid-cols-5">
            <StatCard
              label="Prompts"
              count={stats.total_prompts ?? 0}
              icon={Sparkles}
            />
            <StatCard
              label="Artifacts"
              count={stats.total_artifacts ?? 0}
              icon={FileText}
            />
            <StatCard
              label="Blueprints"
              count={stats.total_blueprints ?? 0}
              icon={FolderKanban}
            />
            <StatCard
              label="Memories"
              count={stats.total_memories ?? 0}
              icon={Sparkles}
            />
            <StatCard
              label="Feed Items"
              count={stats.total_feed_items ?? 0}
              icon={Rss}
            />
          </div>
        </section>
      )}

      {currentTeam && (
        <AccessActivityPanel
          teamId={currentTeam.id}
          resourceType="project"
          resourceId={project.id}
        />
      )}

      {currentTeam && (
        <ResourceCreationChart teamId={currentTeam.id} slug={project.slug} />
      )}

      <ConfirmDialog
        open={!!projectToDelete}
        onOpenChange={open => {
          if (!open) setProjectToDelete(null)
        }}
        title="Delete project?"
        description={
          <>
            This will permanently delete{' '}
            <span className="font-medium">
              {projectToDelete?.name ?? 'this project'}
            </span>
            . Artifacts and blueprints tied to this project may also be
            affected. This action cannot be undone.
          </>
        }
        confirmLabel="Delete"
        variant="destructive"
        loading={deleting}
        onConfirm={handleDelete}
      />
    </div>
  )
}
