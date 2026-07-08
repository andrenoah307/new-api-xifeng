import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueryClient, useMutation } from '@tanstack/react-query'
import type { Row } from '@tanstack/react-table'
import {
  MoreHorizontal,
  Copy,
  FileText,
  Pencil,
  Power,
  Trash2,
} from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { ConfirmDialog } from '@/components/confirm-dialog'
import type { InvitationCode } from '../api'
import {
  updateInvitationCode,
  deleteInvitationCode as deleteInvitationCodeApi,
} from '../api'
import { INVITATION_CODE_STATUS } from '../constants'
import { invitationCodesQueryKeys } from '../lib/invitation-code-actions'
import { useInvitationCodes } from './invitation-codes-provider'

export function DataTableRowActions({ row }: { row: Row<InvitationCode> }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { openEdit, openUsages } = useInvitationCodes()
  const record = row.original
  const isEnabled = record.status === INVITATION_CODE_STATUS.ENABLED

  const [deleteOpen, setDeleteOpen] = useState(false)

  const toggleMutation = useMutation({
    mutationFn: () =>
      updateInvitationCode({
        id: record.id,
        name: record.name,
        status: isEnabled
          ? INVITATION_CODE_STATUS.DISABLED
          : INVITATION_CODE_STATUS.ENABLED,
        max_uses: record.max_uses,
        owner_user_id: record.owner_user_id,
        expired_time: record.expired_time,
      }),
    onSuccess: () => {
      toast.success(t('Operation successful'))
      queryClient.invalidateQueries({
        queryKey: invitationCodesQueryKeys.lists(),
      })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: () => deleteInvitationCodeApi(record.id),
    onSuccess: () => {
      toast.success(t('Deleted successfully'))
      setDeleteOpen(false)
      queryClient.invalidateQueries({
        queryKey: invitationCodesQueryKeys.lists(),
      })
    },
  })

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(record.code).then(
      () => toast.success(t('Copied to clipboard')),
      () => toast.error(t('Copy failed'))
    )
  }, [record.code, t])

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="icon" className="h-8 w-8">
            <MoreHorizontal className="h-4 w-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onClick={handleCopy}>
            <Copy className="mr-2 h-4 w-4" />
            {t('Copy Code')}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => openUsages(record)}>
            <FileText className="mr-2 h-4 w-4" />
            {t('Usage Records')}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => openEdit(record)}>
            <Pencil className="mr-2 h-4 w-4" />
            {t('Edit')}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            onClick={() => toggleMutation.mutate()}
            disabled={toggleMutation.isPending}
          >
            <Power className="mr-2 h-4 w-4" />
            {isEnabled ? t('Disable') : t('Enable')}
          </DropdownMenuItem>
          <DropdownMenuItem
            className="text-destructive"
            onClick={() => setDeleteOpen(true)}
          >
            <Trash2 className="mr-2 h-4 w-4" />
            {t('Delete')}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t('Delete invitation code')}
        desc={t(
          'This action cannot be undone. Are you sure you want to delete this invitation code?'
        )}
        destructive
        handleConfirm={() => deleteMutation.mutate()}
        isLoading={deleteMutation.isPending}
      />
    </>
  )
}
