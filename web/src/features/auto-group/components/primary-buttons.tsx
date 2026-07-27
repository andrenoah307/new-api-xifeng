import { Plus, UserPlus, Zap } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { triggerSweep } from '../api'
import { SUCCESS_MESSAGES } from '../constants'
import { useAutoGroup } from './auto-group-provider'

export function PrimaryButtons() {
  const { t } = useTranslation()
  const { setOpen, activeTab, triggerRefresh } = useAutoGroup()
  const [isSweeping, setIsSweeping] = useState(false)

  const handleSweep = async () => {
    setIsSweeping(true)
    try {
      const result = await triggerSweep()
      if (result.success) {
        toast.success(t(SUCCESS_MESSAGES.SWEEP_TRIGGERED))
        triggerRefresh()
      }
    } catch {
      // HTTP 层错误：拦截器已提示
    } finally {
      setIsSweeping(false)
    }
  }

  if (activeTab === 'enrollments') {
    return (
      <div className='flex gap-2'>
        <Button
          size='sm'
          variant='outline'
          onClick={handleSweep}
          disabled={isSweeping}
        >
          <Zap className='h-4 w-4' />
          {isSweeping ? t('Sweeping...') : t('Trigger Sweep')}
        </Button>
        <Button size='sm' onClick={() => setOpen('enroll')}>
          <UserPlus className='h-4 w-4' />
          {t('Enroll Users')}
        </Button>
      </div>
    )
  }

  return (
    <div className='flex gap-2'>
      <Button size='sm' onClick={() => setOpen('create-rule')}>
        <Plus className='h-4 w-4' />
        {t('Create Rule')}
      </Button>
    </div>
  )
}
