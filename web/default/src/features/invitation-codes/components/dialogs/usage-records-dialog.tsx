import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Skeleton } from '@/components/ui/skeleton'
import { formatTimestampToDate } from '@/lib/format'
import { getInvitationCodeUsages } from '../../api'
import { invitationCodesQueryKeys } from '../../lib/invitation-code-actions'
import { useInvitationCodes } from '../invitation-codes-provider'

export function UsageRecordsDialog() {
  const { t } = useTranslation()
  const { usagesCode, usagesOpen, closeUsages } = useInvitationCodes()

  const { data: usages, isLoading } = useQuery({
    queryKey: invitationCodesQueryKeys.usages(usagesCode?.id ?? 0),
    queryFn: () => getInvitationCodeUsages(usagesCode!.id),
    enabled: usagesOpen && usagesCode !== null,
  })

  return (
    <Dialog open={usagesOpen} onOpenChange={(open) => !open && closeUsages()}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('Usage Records')}</DialogTitle>
          <DialogDescription>
            {usagesCode
              ? t('Usage records for code: {{code}}', {
                  code: usagesCode.code,
                })
              : ''}
          </DialogDescription>
        </DialogHeader>
        <div className="max-h-[60vh] overflow-y-auto">
          {isLoading ? (
            <div className="space-y-2 p-4">
              {[1, 2, 3].map((i) => (
                <Skeleton key={i} className="h-10 w-full" />
              ))}
            </div>
          ) : !usages || usages.length === 0 ? (
            <div className="text-muted-foreground py-12 text-center text-sm">
              {t('No usage records')}
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('User ID')}</TableHead>
                  <TableHead>{t('Username')}</TableHead>
                  <TableHead>{t('Used At')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {usages.map((u) => (
                  <TableRow key={u.id}>
                    <TableCell className="font-mono text-xs">
                      {u.user_id}
                    </TableCell>
                    <TableCell>{u.username || '-'}</TableCell>
                    <TableCell className="text-muted-foreground text-xs">
                      {formatTimestampToDate(u.used_time)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
