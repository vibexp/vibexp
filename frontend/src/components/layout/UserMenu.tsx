import {
  FileText,
  HelpCircle,
  Lock,
  LogOut,
  Monitor,
  Moon,
  Settings as SettingsIcon,
  Shield,
  Sun,
} from 'lucide-react'
import { useNavigate } from 'react-router'

import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useAuth } from '@/contexts/useAuth'
import { useSiteInfo } from '@/features/site-info/useSiteInfo'
import { useTheme } from '@/lib/theme'

/**
 * One of the instance's own links (support, terms, privacy) as a menu item.
 * A real `<a>`, slotted into the item, so it opens in a new tab and still
 * supports middle-click, "copy link" and screen-reader link semantics.
 */
function ExternalMenuLink({
  href,
  icon: Icon,
  children,
}: Readonly<{
  href: string
  icon: typeof HelpCircle
  children: React.ReactNode
}>) {
  return (
    <DropdownMenuItem asChild>
      <a href={href} target="_blank" rel="noopener noreferrer">
        <Icon className="mr-2 size-4" />
        {children}
      </a>
    </DropdownMenuItem>
  )
}

const THEME_CHOICES = [
  { value: 'light', label: 'Light', icon: Sun },
  { value: 'dark', label: 'Dark', icon: Moon },
  { value: 'system', label: 'System', icon: Monitor },
] as const

export function UserMenu() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()
  const { supportHref, terms_url, privacy_url } = useSiteInfo()
  const { theme, setTheme } = useTheme()
  const hasSiteLinks =
    supportHref !== null || terms_url !== null || privacy_url !== null

  if (!user) return null

  const initials = (user.name || user.email || '?')
    .split(' ')
    .map(p => p[0])
    .join('')
    .slice(0, 2)
    .toUpperCase()

  const handleLogout = () => {
    logout()
  }

  const handleSettings = () => {
    void navigate('/settings')
  }

  const handleAdminPortal = () => {
    void navigate('/admin')
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="rounded-full"
          data-testid="user-menu"
        >
          <Avatar className="size-8">
            {user.avatar_url ? <AvatarImage src={user.avatar_url} /> : null}
            <AvatarFallback>{initials}</AvatarFallback>
          </Avatar>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        <DropdownMenuLabel className="flex flex-col">
          <span className="truncate text-sm font-medium">
            {user.name || 'User'}
          </span>
          <span className="text-muted-foreground truncate text-xs font-normal">
            {user.email}
          </span>
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={handleSettings}>
          <SettingsIcon className="mr-2 size-4" />
          Settings
        </DropdownMenuItem>
        {user.is_instance_admin && (
          <DropdownMenuItem onClick={handleAdminPortal}>
            <Shield className="mr-2 size-4" />
            Admin Portal
          </DropdownMenuItem>
        )}
        {/* Appearance is a personal preference set once, so it lives here
            rather than as a button of its own in the header. */}
        <DropdownMenuSeparator />
        <DropdownMenuLabel className="text-muted-foreground text-xs font-normal">
          Theme
        </DropdownMenuLabel>
        <DropdownMenuRadioGroup
          value={theme}
          onValueChange={value => {
            setTheme(value as typeof theme)
          }}
        >
          {THEME_CHOICES.map(({ value, label, icon: Icon }) => (
            <DropdownMenuRadioItem
              key={value}
              value={value}
              // Choosing a theme is not leaving the menu: it stays open so
              // the change can be seen and undone.
              onSelect={event => {
                event.preventDefault()
              }}
            >
              <Icon className="mr-2 size-4" />
              {label}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
        {/* The instance's own links, from site info. Each shows only when it
            is configured; with none, the menu has no extra group at all. */}
        {hasSiteLinks && <DropdownMenuSeparator />}
        {supportHref !== null && (
          <ExternalMenuLink href={supportHref} icon={HelpCircle}>
            Support
          </ExternalMenuLink>
        )}
        {terms_url !== null && (
          <ExternalMenuLink href={terms_url} icon={FileText}>
            Terms
          </ExternalMenuLink>
        )}
        {privacy_url !== null && (
          <ExternalMenuLink href={privacy_url} icon={Lock}>
            Privacy
          </ExternalMenuLink>
        )}
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={handleLogout}>
          <LogOut className="mr-2 size-4" />
          Sign out
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
