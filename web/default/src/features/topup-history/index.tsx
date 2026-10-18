import { getRouteApi, useNavigate } from '@tanstack/react-router'
import { FileText } from 'lucide-react'
import { lazy, Suspense, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useIsAdmin } from '@/hooks/use-admin'
import { hasPermission } from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'

import { CreateInvoiceTicketDialog } from '../tickets/components/dialogs/create-invoice-ticket-dialog'
import { TopupTable } from './components/topup-table'

const route = getRouteApi('/_authenticated/topup-history/')
const FinanceRecords = lazy(() => import('../finance'))

export default function TopupHistoryPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { applyInvoice, view } = route.useSearch()
  const user = useAuthStore((state) => state.auth.user)
  const admin = useIsAdmin()
  const canReadFinance = hasPermission(user, 'finance', 'read')
  const finance =
    canReadFinance && !applyInvoice && (view === 'finance' || (!view && !admin))
  const [invoiceOpen, setInvoiceOpen] = useState(false)

  useEffect(() => {
    if (!applyInvoice) return
    setInvoiceOpen(true)
    // 消费掉一次性参数，避免刷新/返回时再次弹窗
    void navigate({
      to: '/topup-history',
      search: (prev) => ({ ...prev, view: 'records', applyInvoice: undefined }),
      replace: true,
    })
  }, [applyInvoice, navigate])

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Top-up History')}</SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {t('View top-up and payment records')}
        </SectionPageLayout.Description>
        <SectionPageLayout.Actions>
          <Button
            variant='outline'
            size='sm'
            onClick={() => setInvoiceOpen(true)}
          >
            <FileText className='mr-1.5 size-4' />
            {t('Apply for Invoice')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          {canReadFinance && (
            <Tabs
              value={finance ? 'finance' : 'records'}
              onValueChange={(value) => {
                void navigate({
                  to: '/topup-history',
                  search: (prev) => ({
                    ...prev,
                    view: value === 'finance' ? 'finance' : 'records',
                  }),
                })
              }}
              className='mb-4'
            >
              <TabsList>
                <TabsTrigger value='records'>{t('Top-up History')}</TabsTrigger>
                <TabsTrigger value='finance'>{t('Finance')}</TabsTrigger>
              </TabsList>
            </Tabs>
          )}
          {finance ? (
            <Suspense fallback={<p>{t('Loading...')}</p>}>
              <FinanceRecords key={user?.id} />
            </Suspense>
          ) : (
            <TopupTable key={`${user?.id}-${admin}`} />
          )}
        </SectionPageLayout.Content>
      </SectionPageLayout>
      <CreateInvoiceTicketDialog
        open={invoiceOpen}
        onOpenChange={setInvoiceOpen}
        onCreated={(id) =>
          navigate({
            to: '/tickets/$ticketId',
            params: { ticketId: String(id) },
          })
        }
      />
    </>
  )
}
