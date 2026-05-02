import { useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, RefreshCw, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { clearInvalidInvitationCodes } from '../api'
import { invitationCodesQueryKeys } from '../lib/invitation-code-actions'
import { useInvitationCodes } from './invitation-codes-provider'

export function InvitationCodesPrimaryButtons() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { openCreate } = useInvitationCodes()
  const [clearOpen, setClearOpen] = useState(false)

  const clearMutation = useMutation({
    mutationFn: clearInvalidInvitationCodes,
    onSuccess: (count) => {
      setClearOpen(false)
      toast.success(
        t('Deleted {{count}} invalid invitation codes', { count })
      )
      queryClient.invalidateQueries({
        queryKey: invitationCodesQueryKeys.lists(),
      })
    },
  })

  const handleRefresh = useCallback(() => {
    queryClient.invalidateQueries({
      queryKey: invitationCodesQueryKeys.lists(),
    })
  }, [queryClient])

  return (
    <div className="flex items-center gap-2">
      <Button onClick={openCreate} size="sm">
        <Plus className="mr-1.5 h-4 w-4" />
        {t('Create')}
      </Button>
      <Button variant="outline" size="sm" onClick={handleRefresh}>
        <RefreshCw className="mr-1.5 h-4 w-4" />
        {t('Refresh')}
      </Button>
      <Button
        variant="outline"
        size="sm"
        onClick={() => setClearOpen(true)}
      >
        <Trash2 className="mr-1.5 h-4 w-4" />
        {t('Clear Invalid')}
      </Button>
      <ConfirmDialog
        open={clearOpen}
        onOpenChange={setClearOpen}
        title={t('Clear invalid invitation codes')}
        desc={t(
          'This will permanently delete all disabled, expired, and exhausted invitation codes. Continue?'
        )}
        destructive
        handleConfirm={() => clearMutation.mutate()}
        isLoading={clearMutation.isPending}
      />
    </div>
  )
}
