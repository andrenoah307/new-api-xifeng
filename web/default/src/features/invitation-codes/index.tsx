import { useTranslation } from 'react-i18next'
import { SectionPageLayout } from '@/components/layout'
import { InvitationCodesProvider } from './components/invitation-codes-provider'
import { InvitationCodesTable } from './components/invitation-codes-table'
import { InvitationCodesPrimaryButtons } from './components/invitation-codes-primary-buttons'
import { EditInvitationCodeSheet } from './components/dialogs/edit-invitation-code-sheet'
import { UsageRecordsDialog } from './components/dialogs/usage-records-dialog'

export default function InvitationCodesPage() {
  const { t } = useTranslation()

  return (
    <InvitationCodesProvider>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {t('Invitation Codes')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {t('Manage invitation codes for user registration and top-up')}
        </SectionPageLayout.Description>
        <SectionPageLayout.Actions>
          <InvitationCodesPrimaryButtons />
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <InvitationCodesTable />
        </SectionPageLayout.Content>
      </SectionPageLayout>
      <EditInvitationCodeSheet />
      <UsageRecordsDialog />
    </InvitationCodesProvider>
  )
}
