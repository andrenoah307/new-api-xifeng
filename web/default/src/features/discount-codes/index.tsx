import { useTranslation } from 'react-i18next'
import { SectionPageLayout } from '@/components/layout'
import { DiscountCodesDialogs } from './components/discount-codes-dialogs'
import { DiscountCodesPrimaryButtons } from './components/discount-codes-primary-buttons'
import { DiscountCodesProvider } from './components/discount-codes-provider'
import { DiscountCodesTable } from './components/discount-codes-table'

export function DiscountCodes() {
  const { t } = useTranslation()
  return (
    <DiscountCodesProvider>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {t('Discount Codes')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {t('Manage discount codes for payment discounts')}
        </SectionPageLayout.Description>
        <SectionPageLayout.Actions>
          <DiscountCodesPrimaryButtons />
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <DiscountCodesTable />
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <DiscountCodesDialogs />
    </DiscountCodesProvider>
  )
}
