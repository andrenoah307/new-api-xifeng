import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { SectionPageLayout } from '@/components/layout'
import { DistributionTab } from './distribution-tab'
import { ModerationTab } from './moderation-tab'
import { EnforcementTab } from './enforcement-tab'

export function RiskTabs() {
  const { t } = useTranslation()
  const [tab, setTab] = useState('distribution')

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Risk Control')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t(
          'Manage and monitor risk control rules, subjects, and incidents'
        )}
      </SectionPageLayout.Description>
      <SectionPageLayout.Content>
        <Tabs value={tab} onValueChange={setTab}>
          <TabsList>
            <TabsTrigger value="distribution">
              {t('Distribution Detection')}
            </TabsTrigger>
            <TabsTrigger value="moderation">
              {t('Content Moderation')}
            </TabsTrigger>
            <TabsTrigger value="enforcement">
              {t('Enforcement')}
            </TabsTrigger>
          </TabsList>
          <TabsContent value="distribution" className="mt-4">
            <DistributionTab />
          </TabsContent>
          <TabsContent value="moderation" className="mt-4">
            <ModerationTab />
          </TabsContent>
          <TabsContent value="enforcement" className="mt-4">
            <EnforcementTab />
          </TabsContent>
        </Tabs>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
