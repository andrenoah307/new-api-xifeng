import { useTranslation } from 'react-i18next'
import { SectionPageLayout } from '@/components/layout'
import { TopupTable } from './components/topup-table'

export default function TopupHistoryPage() {
  const { t } = useTranslation()

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Top-up History')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t('View top-up and payment records')}
      </SectionPageLayout.Description>
      <SectionPageLayout.Content>
        <TopupTable />
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
