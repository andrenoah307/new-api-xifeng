import { Card } from '@/components/ui/card'

export function OverviewCard({
  title,
  value,
  extra,
}: {
  title: string
  value: React.ReactNode
  extra?: React.ReactNode
}) {
  return (
    <Card className="min-h-[120px] p-4">
      <p className="text-muted-foreground text-xs">{title}</p>
      <div className="mt-2 text-3xl font-bold">{value}</div>
      {extra && (
        <p className="text-muted-foreground mt-2 text-xs">{extra}</p>
      )}
    </Card>
  )
}
