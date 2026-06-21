import { Check, Copy } from 'lucide-react'
import { useCallback } from 'react'

import { Button } from '@/components/ui/button'
import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'
import { cn } from '@/lib/utils'

interface CopyButtonProps {
  value: string
  size?: 'sm' | 'default' | 'lg' | 'icon'
  variant?: 'default' | 'ghost' | 'outline' | 'secondary'
  label?: string
  className?: string
  testId?: string
}

export function CopyButton({
  value,
  size = 'icon',
  variant = 'ghost',
  label,
  className,
  testId,
}: CopyButtonProps) {
  const { copied, copy } = useCopyToClipboard()

  const handleCopy = useCallback(() => {
    copy(value)
  }, [copy, value])

  const Icon = copied ? Check : Copy

  return (
    <Button
      type="button"
      size={size}
      variant={variant}
      onClick={handleCopy}
      aria-label={label ?? 'Copy to clipboard'}
      className={cn(className)}
      data-testid={testId}
    >
      <Icon className="size-4" />
      {size !== 'icon' && label && <span>{copied ? 'Copied!' : label}</span>}
    </Button>
  )
}
