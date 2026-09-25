/** A titled block of admin insight panels, shared by the dashboard and user detail. */
export function Section({
  title,
  description,
  children,
}: Readonly<{
  title: string
  description?: string
  children: React.ReactNode
}>) {
  return (
    <section className="space-y-3">
      <div>
        <h2 className="text-sm font-semibold">{title}</h2>
        {description && (
          <p className="text-muted-foreground text-xs">{description}</p>
        )}
      </div>
      {children}
    </section>
  )
}
