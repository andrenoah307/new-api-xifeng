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
import { unenrollUser } from '../api'
import { SUCCESS_MESSAGES } from '../constants'
import { useAutoGroup } from './auto-group-provider'

export function EnrollmentDeleteDialog() {
  const { t } = useTranslation()
  const { open, setOpen, currentEnrollment, triggerRefresh } = useAutoGroup()
  const [isDeleting, setIsDeleting] = useState(false)

  const handleUnenroll = async () => {
    if (!currentEnrollment) return
    setIsDeleting(true)
    try {
      const result = await unenrollUser(currentEnrollment.id)
      if (result.success) {
        toast.success(t(SUCCESS_MESSAGES.USER_UNENROLLED))
        setOpen(null)
        triggerRefresh()
      }
    } catch {
      // HTTP 层错误：拦截器已提示
    } finally {
      setIsDeleting(false)
    }
  }

  return (
    <AlertDialog
      open={open === 'unenroll'}
      onOpenChange={(open) => !open && setOpen(null)}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t('Unenroll User?')}</AlertDialogTitle>
          <AlertDialogDescription>
            {t('This will unenroll')}{' '}
            <span className='font-semibold'>
              {currentEnrollment?.username}
            </span>{' '}
            {t(
              'and revert their group back to the original group. This action cannot be undone.'
            )}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={isDeleting}>
            {t('Cancel')}
          </AlertDialogCancel>
          <AlertDialogAction
            onClick={handleUnenroll}
            disabled={isDeleting}
            className='bg-destructive text-destructive-foreground hover:bg-destructive/90'
          >
            {isDeleting ? t('Unenrolling...') : t('Unenroll')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
