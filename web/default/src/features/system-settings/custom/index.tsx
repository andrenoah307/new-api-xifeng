import { useParams } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useSystemOptions } from '../hooks/use-system-options'
import {
  CUSTOM_DEFAULT_SECTION,
  getCustomSectionContent,
  type CustomSectionId,
  type CustomSettingsData,
} from './section-registry'

export function CustomSettings() {
  const { t } = useTranslation()
  const { data, isLoading } = useSystemOptions()
  const params = useParams({
    from: '/_authenticated/system-settings/custom/$section' as const,
  })

  if (isLoading) {
    return (
      <div className='flex items-center justify-center py-12'>
        <div className='text-muted-foreground'>{t('Loading settings...')}</div>
      </div>
    )
  }

  const settings: CustomSettingsData = {}
  if (data?.data) {
    for (const opt of data.data) {
      settings[opt.key] = opt.value
    }
  }

  const activeSection = (
    params?.section ?? CUSTOM_DEFAULT_SECTION
  ) as CustomSectionId

  return (
    <div className='flex h-full w-full flex-1 flex-col'>
      <div className='faded-bottom h-full w-full overflow-y-auto scroll-smooth pe-4 pb-12'>
        <div className='space-y-4'>
          {getCustomSectionContent(activeSection, settings)}
        </div>
      </div>
    </div>
  )
}
