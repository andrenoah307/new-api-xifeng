import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { FileText } from 'lucide-react'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { TopupTable } from './components/topup-table'
import { CreateInvoiceTicketDialog } from '../tickets/components/dialogs/create-invoice-ticket-dialog'

export default function TopupHistoryPage() {
  const { t } = useTranslation()
  const [invoiceOpen, setInvoiceOpen] = useState(false)

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Top-up History')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t('View top-up and payment records')}
      </SectionPageLayout.Description>
      <SectionPageLayout.Actions>
        <Button variant="outline" size="sm" onClick={() => setInvoiceOpen(true)}>
          <FileText className="mr-1.5 size-4" />
          {t('Apply for Invoice')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <TopupTable />
      </SectionPageLayout.Content>
      <CreateInvoiceTicketDialog
        open={invoiceOpen}
        onOpenChange={setInvoiceOpen}
      />
    </SectionPageLayout>
  )
}
