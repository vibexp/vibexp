import type { ColumnDef } from '@tanstack/react-table'
import { Calendar, Mail, Shield, Trash2, User } from 'lucide-react'
import { useState } from 'react'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { DataTable } from '@/components/DataTable'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import type { TeamMember } from '@/types'

interface TeamMembersListProps {
  members: TeamMember[] | undefined
  isOwner?: boolean
  onRemoveMember?: (userId: string) => Promise<void>
}

const formatDate = (dateString: string) =>
  new Date(dateString).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })

const roleVariant = (role: string) => {
  switch (role) {
    case 'owner':
      return 'default'
    case 'admin':
      return 'secondary'
    default:
      return 'outline'
  }
}

const capitalize = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)

function buildColumns({
  isOwner,
  onRemoveClick,
}: {
  isOwner: boolean
  onRemoveClick?: (userId: string) => void
}): ColumnDef<TeamMember>[] {
  const columns: ColumnDef<TeamMember>[] = [
    {
      accessorKey: 'name',
      header: 'Member',
      cell: ({ row }) => {
        const member = row.original
        return (
          <div className="flex items-center gap-3">
            <Avatar className="size-9">
              <AvatarFallback>
                {member.name.charAt(0).toUpperCase()}
              </AvatarFallback>
            </Avatar>
            <div>
              <div className="flex items-center gap-2">
                <User className="text-muted-foreground size-4" />
                <span className="font-medium">{member.name}</span>
              </div>
              <div className="text-muted-foreground flex items-center gap-2 text-sm">
                <Mail className="size-3.5" />
                <span>{member.email}</span>
              </div>
            </div>
          </div>
        )
      },
    },
    {
      accessorKey: 'role',
      header: 'Role',
      cell: ({ row }) => (
        <div className="flex items-center gap-2">
          <Shield className="text-muted-foreground size-4" />
          <Badge variant={roleVariant(row.original.role)}>
            {capitalize(row.original.role)}
          </Badge>
        </div>
      ),
    },
  ]

  if (isOwner) {
    columns.push({
      accessorKey: 'invitation_status',
      header: 'Status',
      cell: ({ row }) => {
        const status = row.original.invitation_status
        const accepted = !status || status === 'accepted'
        return (
          <Badge variant={accepted ? 'secondary' : 'outline'}>
            {accepted ? 'Accepted' : 'Pending'}
          </Badge>
        )
      },
    })
  }

  columns.push({
    accessorKey: 'joined_at',
    header: 'Joined',
    cell: ({ row }) => {
      const isPending = row.original.invitation_status === 'pending'
      const date = formatDate(row.original.joined_at)
      return (
        <div className="text-muted-foreground flex items-center gap-2 text-sm">
          <Calendar className="size-4" />
          <span>{isPending ? `Invited ${date}` : date}</span>
        </div>
      )
    },
  })

  if (isOwner && onRemoveClick) {
    columns.push({
      id: 'actions',
      enableHiding: false,
      cell: ({ row }) => {
        // Pending invitation rows have a synthetic user_id and no backend
        // membership to remove — suppress the action so an owner can't trigger
        // a 404 against /teams/{id}/members/{userId}. Revoking invitations is
        // a separate flow, out of scope for this row.
        if (row.original.role === 'owner') return null
        if (row.original.invitation_status === 'pending') return null
        return (
          <div className="flex justify-end">
            <Button
              variant="ghost"
              size="icon"
              aria-label={`Remove ${row.original.name}`}
              onClick={e => {
                e.stopPropagation()
                onRemoveClick(row.original.user_id)
              }}
            >
              <Trash2 className="size-4" />
            </Button>
          </div>
        )
      },
    })
  }

  return columns
}

export function TeamMembersList({
  members,
  isOwner = false,
  onRemoveMember,
}: TeamMembersListProps) {
  const [isRemoveDialogOpen, setIsRemoveDialogOpen] = useState(false)
  const [selectedMember, setSelectedMember] = useState<TeamMember | null>(null)
  const [isRemoving, setIsRemoving] = useState(false)

  const handleRemoveClick = (userId: string) => {
    const member = members?.find(m => m.user_id === userId)
    if (member) {
      setSelectedMember(member)
      setIsRemoveDialogOpen(true)
    }
  }

  const handleConfirmRemove = async () => {
    if (!selectedMember || !onRemoveMember) return

    try {
      setIsRemoving(true)
      await onRemoveMember(selectedMember.user_id)
      setIsRemoveDialogOpen(false)
      setSelectedMember(null)
    } catch {
      // error handled upstream
    } finally {
      setIsRemoving(false)
    }
  }

  const columns = buildColumns({
    isOwner,
    onRemoveClick: isOwner && onRemoveMember ? handleRemoveClick : undefined,
  })

  if (!members || members.length === 0) {
    return (
      <Card>
        <CardContent className="py-8 text-center">
          <p className="text-muted-foreground text-sm">No members found</p>
        </CardContent>
      </Card>
    )
  }

  return (
    <>
      <Card>
        <CardContent className="p-4">
          <DataTable columns={columns} data={members} />
        </CardContent>
      </Card>

      <ConfirmDialog
        open={isRemoveDialogOpen}
        onOpenChange={open => {
          if (!open) {
            setIsRemoveDialogOpen(false)
            setSelectedMember(null)
          }
        }}
        title="Remove team member?"
        description={
          <>
            Are you sure you want to remove{' '}
            <span className="font-medium">
              {selectedMember?.name ?? 'this member'}
            </span>{' '}
            from the team? This action cannot be undone.
          </>
        }
        confirmLabel="Remove"
        variant="destructive"
        loading={isRemoving}
        onConfirm={handleConfirmRemove}
      />
    </>
  )
}
