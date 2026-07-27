import { useTranslation } from 'react-i18next'
import { SectionPageLayout } from '@/components/layout'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { AutoGroupDialogs } from './components/auto-group-dialogs'
import { AutoGroupProvider, useAutoGroup } from './components/auto-group-provider'
import { EnrollmentsTable } from './components/enrollments-table'
import { PrimaryButtons } from './components/primary-buttons'
import { RulesTable } from './components/rules-table'

function AutoGroupContent() {
  const { t } = useTranslation()
  const { activeTab, setActiveTab } = useAutoGroup()

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Auto Group')}</SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t('Manage automatic user group assignment rules and enrollments')}
      </SectionPageLayout.Description>
      <SectionPageLayout.Actions>
        <PrimaryButtons />
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <Tabs value={activeTab} onValueChange={setActiveTab}>
          <TabsList>
            <TabsTrigger value='rules'>{t('Rules')}</TabsTrigger>
            <TabsTrigger value='enrollments'>
              {t('Enrollments')}
            </TabsTrigger>
          </TabsList>
          <TabsContent value='rules'>
            <RulesTable />
          </TabsContent>
          <TabsContent value='enrollments'>
            <EnrollmentsTable />
          </TabsContent>
        </Tabs>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

export function AutoGroup() {
  return (
    <AutoGroupProvider>
      <AutoGroupContent />
      <AutoGroupDialogs />
    </AutoGroupProvider>
  )
}
