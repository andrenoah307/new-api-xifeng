import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { getRouteApi, useNavigate } from '@tanstack/react-router'
import { FileText } from 'lucide-react'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { TopupTable } from './components/topup-table'
import { CreateInvoiceTicketDialog } from '../tickets/components/dialogs/create-invoice-ticket-dialog'

const route = getRouteApi('/_authenticated/topup-history/')

export default function TopupHistoryPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { applyInvoice } = route.useSearch()
  const [invoiceOpen, setInvoiceOpen] = useState(false)

  useEffect(() => {
    if (!applyInvoice) return
    setInvoiceOpen(true)
    // 消费掉一次性参数，避免刷新/返回时再次弹窗
    void navigate({
      to: '/topup-history',
      search: (prev) => ({ ...prev, applyInvoice: undefined }),
      replace: true,
    })
  }, [applyInvoice, navigate])

  return (
    <>
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
      </SectionPageLayout>
      <CreateInvoiceTicketDialog
        open={invoiceOpen}
        onOpenChange={setInvoiceOpen}
      />
    </>
  )
}
