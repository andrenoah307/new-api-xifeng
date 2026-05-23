import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { deleteRule } from '../api'
import { SUCCESS_MESSAGES } from '../constants'
import { useAutoGroup } from './auto-group-provider'

export function RuleDeleteDialog() {
  const { t } = useTranslation()
  const { open, setOpen, currentRule, triggerRefresh } = useAutoGroup()
  const [isDeleting, setIsDeleting] = useState(false)

  const handleDelete = async () => {
    if (!currentRule) return
    setIsDeleting(true)
    try {
      const result = await deleteRule(currentRule.id)
      if (result.success) {
        toast.success(t(SUCCESS_MESSAGES.RULE_DELETED))
        setOpen(null)
        triggerRefresh()
      }
    } finally {
      setIsDeleting(false)
    }
  }

  return (
    <AlertDialog
      open={open === 'delete-rule'}
      onOpenChange={(open) => !open && setOpen(null)}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t('Are you sure?')}</AlertDialogTitle>
          <AlertDialogDescription>
            {t('This will permanently delete rule')}{' '}
            <span className='font-semibold'>{currentRule?.name}</span>
            {t('. This action cannot be undone.')}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={isDeleting}>
            {t('Cancel')}
          </AlertDialogCancel>
          <AlertDialogAction
            onClick={handleDelete}
            disabled={isDeleting}
            className='bg-destructive text-destructive-foreground hover:bg-destructive/90'
          >
            {isDeleting ? t('Deleting...') : t('Delete')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
