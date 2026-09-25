/** A full-width section heading inside an Advanced filters panel grid. */
export function GroupHeading({ children }: Readonly<{ children: string }>) {
  return (
    <h3 className="text-muted-foreground col-span-full text-xs font-semibold uppercase tracking-wide">
      {children}
    </h3>
  )
}
