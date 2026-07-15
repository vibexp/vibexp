import type { ColumnDef } from '@tanstack/react-table'
import { Calendar, Mail, Shield, Trash2, User } from 'lucide-react'
import { useState } from 'react'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { DataTable } from '@/components/DataTable'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { ChangeableTeamRole, TeamMember } from '@/services/teamService'

interface TeamMembersListProps {
  members: TeamMember[] | undefined
  /** Grants the member↔admin role dropdown (`member.role.update`). */
  canManageRoles?: boolean
  /** Grants the remove action (`member.remove`). */
  canRemoveMember?: boolean
  onRemoveMember?: (userId: string) => Promise<void>
  onChangeRole?: (userId: string, role: ChangeableTeamRole) => Promise<void>
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

/**
 * The role cell for a member whose role the caller may change.
 *
 * Owners never reach this component: ownership moves only through transfer, so
 * offering "owner" here would be a dead option the API rejects.
 */
function RoleSelect({
  member,
  onChangeRole,
}: {
  member: TeamMember
  onChangeRole: (userId: string, role: ChangeableTeamRole) => void
}) {
  return (
    <div className="flex items-center gap-2">
      <Shield className="text-muted-foreground size-4" />
      <Select
        value={member.role}
        onValueChange={value => {
          if (value !== member.role) {
            onChangeRole(member.user_id, value as ChangeableTeamRole)
          }
        }}
      >
        <SelectTrigger
          className="h-8 w-28"
          aria-label={`Change role for ${member.name}`}
          // The row is a click target; opening the picker must not also fire it.
          onClick={e => {
            e.stopPropagation()
          }}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="member">Member</SelectItem>
          <SelectItem value="admin">Admin</SelectItem>
        </SelectContent>
      </Select>
    </div>
  )
}

function buildColumns({
  managesMembers,
  canManageRoles,
  onRemoveClick,
  onChangeRole,
}: {
  managesMembers: boolean
  canManageRoles: boolean
  onRemoveClick?: (userId: string) => void
  onChangeRole?: (userId: string, role: ChangeableTeamRole) => void
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
      cell: ({ row }) => {
        const member = row.original
        // The owner's role is immutable here (transfer ownership instead), and
        // a pending invitee has no membership to update yet — both stay badges.
        const editable =
          canManageRoles &&
          onChangeRole &&
          member.role !== 'owner' &&
          member.invitation_status !== 'pending'

        if (editable) {
          return <RoleSelect member={member} onChangeRole={onChangeRole} />
        }

        return (
          <div className="flex items-center gap-2">
            <Shield className="text-muted-foreground size-4" />
            <Badge variant={roleVariant(member.role)}>
              {capitalize(member.role)}
            </Badge>
          </div>
        )
      },
    },
  ]

  if (managesMembers) {
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

  if (onRemoveClick) {
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
  canManageRoles = false,
  canRemoveMember = false,
  onRemoveMember,
  onChangeRole,
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

  const handleChangeRole = (userId: string, role: ChangeableTeamRole) => {
    // The caller owns the members state and reverts on failure (#225), so a
    // rejection here is already surfaced upstream.
    void onChangeRole?.(userId, role)
  }

  // Invitation status is member-management detail: show it to whoever manages
  // members. Previously owner-only, which left admins unable to see who had
  // actually accepted despite being able to invite and remove.
  const managesMembers = canManageRoles || canRemoveMember

  const columns = buildColumns({
    managesMembers,
    canManageRoles,
    onRemoveClick:
      canRemoveMember && onRemoveMember ? handleRemoveClick : undefined,
    onChangeRole: canManageRoles && onChangeRole ? handleChangeRole : undefined,
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
