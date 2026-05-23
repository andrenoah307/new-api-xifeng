import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { enrollUsers } from '../api'
import { SUCCESS_MESSAGES } from '../constants'
import { useAutoGroup } from './auto-group-provider'

export function EnrollDialog() {
  const { t } = useTranslation()
  const { open, setOpen, triggerRefresh } = useAutoGroup()
  const [userIdsInput, setUserIdsInput] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  const handleSubmit = async () => {
    const ids = userIdsInput
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
      .map(Number)
      .filter((n) => !isNaN(n) && n > 0)

    if (ids.length === 0) {
      toast.error(t('Please enter at least one valid user ID'))
      return
    }

    setIsSubmitting(true)
    try {
      const result = await enrollUsers(ids)
      if (result.success) {
        const created = result.data?.created ?? 0
        const errors = result.data?.errors
        toast.success(
          t(SUCCESS_MESSAGES.USERS_ENROLLED) +
            (created > 0 ? ` (${created})` : '')
        )
        if (errors && errors.length > 0) {
          toast.warning(errors.join(', '))
        }
        setOpen(null)
        setUserIdsInput('')
        triggerRefresh()
      }
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Dialog
      open={open === 'enroll'}
      onOpenChange={(isOpen) => {
        if (!isOpen) {
          setOpen(null)
          setUserIdsInput('')
        }
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('Enroll Users')}</DialogTitle>
          <DialogDescription>
            {t(
              'Enter user IDs to enroll them in auto group. Their groups will be evaluated and assigned automatically.'
            )}
          </DialogDescription>
        </DialogHeader>
        <div className='space-y-4 py-2'>
          <div className='space-y-2'>
            <Label>{t('User IDs')}</Label>
            <Input
              value={userIdsInput}
              onChange={(e) => setUserIdsInput(e.target.value)}
              placeholder={t('e.g. 1, 2, 3')}
            />
            <p className='text-muted-foreground text-xs'>
              {t('Comma-separated list of user IDs')}
            </p>
          </div>
        </div>
        <DialogFooter>
          <DialogClose render={<Button variant='outline' />}>
            {t('Cancel')}
          </DialogClose>
          <Button onClick={handleSubmit} disabled={isSubmitting}>
            {isSubmitting ? t('Enrolling...') : t('Enroll')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
