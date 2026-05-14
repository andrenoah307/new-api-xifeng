import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { useDiscountCodes } from './discount-codes-provider'

export function DiscountCodesPrimaryButtons() {
  const { t } = useTranslation()
  const { setOpen } = useDiscountCodes()
  return (
    <div className='flex gap-2'>
      <Button size='sm' onClick={() => setOpen('create')}>
        <Plus className='h-4 w-4' />
        {t('Create Code')}
      </Button>
    </div>
  )
}
