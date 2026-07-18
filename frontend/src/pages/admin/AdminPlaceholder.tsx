/**
 * Temporary placeholder for the admin page bodies. The real Stats dashboard and
 * the Users/Teams list + detail pages land in #316; this shell issue (#315) only
 * ships the entry point, guard, routing, and layout.
 */
export function AdminPlaceholder({ title }: Readonly<{ title: string }>) {
  return (
    <div
      data-testid="admin-placeholder"
      className="text-muted-foreground rounded-lg border border-dashed p-8 text-sm"
    >
      {title} — coming soon.
    </div>
  )
}
