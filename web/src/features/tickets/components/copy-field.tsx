import { CopyButton } from '@/components/copy-button'
import { cn } from '@/lib/utils'

/**
 * 工单详情（发票 / 退款）中的可复制字段：管理员需要把抬头、税号、账号等
 * 逐项粘贴到开票或打款系统，因此每项值都带一个复制按钮。
 */
export function CopyField({
  label,
  value,
  copyValue,
  className,
  valueClassName,
  multiline,
}: {
  label: string
  value: string
  copyValue?: string
  className?: string
  valueClassName?: string
  multiline?: boolean
}) {
  const textToCopy = copyValue ?? value
  return (
    <div className={className}>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className={cn('flex gap-1', multiline ? 'items-start' : 'items-center')}>
        <span
          className={cn(
            'font-medium',
            multiline && 'break-words whitespace-pre-wrap',
            valueClassName
          )}
        >
          {value || '-'}
        </span>
        {textToCopy && (
          <CopyButton
            value={textToCopy}
            size="icon"
            className="h-6 w-6"
            iconClassName="h-3 w-3"
          />
        )}
      </dd>
    </div>
  )
}
