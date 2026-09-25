import { Columns2 } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import type { ColumnVisibility } from '@/pages/admin/users/userColumns'
import { CHOOSABLE_COLUMNS } from '@/pages/admin/users/userColumns'

export interface UserColumnChooserProps {
  visibility: ColumnVisibility
  onVisibleChange: (id: string, visible: boolean) => void
  /** The active sort column, which cannot be hidden while it sorts. */
  sortBy: string
}

export function UserColumnChooser({
  visibility,
  onVisibleChange,
  sortBy,
}: Readonly<UserColumnChooserProps>) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline">
          <Columns2 className="mr-2 size-4" aria-hidden />
          Columns
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuLabel>Activity columns</DropdownMenuLabel>
        <DropdownMenuSeparator />
        {CHOOSABLE_COLUMNS.map(col => (
          <DropdownMenuCheckboxItem
            key={col.id}
            checked={visibility[col.id]}
            disabled={col.id === sortBy}
            onCheckedChange={checked => {
              onVisibleChange(col.id, checked)
            }}
          >
            {col.header}
          </DropdownMenuCheckboxItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
