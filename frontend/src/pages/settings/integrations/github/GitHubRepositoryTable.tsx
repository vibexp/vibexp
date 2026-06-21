import { ExternalLink, Lock, Unlock } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { GitHubRepository } from '@/types/github'

import { ImportBlueprintsButton } from './ImportBlueprintsButton'
import { ImportProjectButton } from './ImportProjectButton'

interface GitHubRepositoryTableProps {
  repositories: GitHubRepository[]
  currentPage: number
  totalPages: number
  onPageChange: (page: number) => void
}

export function GitHubRepositoryTable({
  repositories,
  currentPage,
  totalPages,
  onPageChange,
}: GitHubRepositoryTableProps) {
  return (
    <div className="space-y-3">
      <div className="rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Repository</TableHead>
              <TableHead>Description</TableHead>
              <TableHead>Visibility</TableHead>
              <TableHead className="text-center">Link</TableHead>
              <TableHead className="text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {repositories.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={5}
                  className="text-muted-foreground h-24 text-center"
                >
                  No repositories match the selected filters
                </TableCell>
              </TableRow>
            ) : (
              repositories.map(repo => (
                <TableRow key={repo.id}>
                  <TableCell>
                    <div>
                      <div className="font-medium">{repo.name}</div>
                      <div className="text-muted-foreground text-xs">
                        {repo.full_name}
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="text-muted-foreground max-w-md truncate text-sm">
                      {repo.description ?? (
                        <span className="italic">No description</span>
                      )}
                    </div>
                  </TableCell>
                  <TableCell>
                    {repo.private ? (
                      <Badge variant="outline" className="gap-1">
                        <Lock className="size-3" />
                        Private
                      </Badge>
                    ) : (
                      <Badge variant="outline" className="gap-1">
                        <Unlock className="size-3" />
                        Public
                      </Badge>
                    )}
                  </TableCell>
                  <TableCell className="text-center">
                    <a
                      href={repo.html_url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="text-primary inline-flex items-center hover:underline"
                      aria-label={`Open ${repo.name} on GitHub`}
                    >
                      <ExternalLink className="size-4" />
                    </a>
                  </TableCell>
                  <TableCell>
                    <div className="flex justify-end gap-2">
                      <ImportProjectButton repository={repo} />
                      <ImportBlueprintsButton repository={repo} />
                    </div>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      {totalPages > 1 && (
        <div className="flex items-center justify-between gap-2">
          <div className="text-muted-foreground text-sm">
            Page {currentPage} of {totalPages}
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                onPageChange(currentPage - 1)
              }}
              disabled={currentPage <= 1}
            >
              Previous
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                onPageChange(currentPage + 1)
              }}
              disabled={currentPage >= totalPages}
            >
              Next
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
