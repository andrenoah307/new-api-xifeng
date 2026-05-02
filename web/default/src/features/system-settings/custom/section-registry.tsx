import { createSectionRegistry } from '../utils/section-registry'
import { EmailTemplateSettingsSection } from './email-template-settings-section'
import { TicketSettingsSection } from './ticket-settings-section'
import { GroupMonitoringSettingsSection } from './group-monitoring-settings-section'

export type CustomSettingsData = Record<string, string>

const CUSTOM_SECTIONS = [
  {
    id: 'email-templates',
    titleKey: 'Email Templates',
    descriptionKey: 'Configure email notification templates',
    build: () => <EmailTemplateSettingsSection />,
  },
  {
    id: 'tickets',
    titleKey: 'Ticket Settings',
    descriptionKey: 'Configure ticket system settings',
    build: (_settings: CustomSettingsData) => (
      <TicketSettingsSection settings={_settings} />
    ),
  },
  {
    id: 'group-monitoring',
    titleKey: 'Group Monitoring Settings',
    descriptionKey: 'Configure group monitoring parameters',
    build: (_settings: CustomSettingsData) => (
      <GroupMonitoringSettingsSection settings={_settings} />
    ),
  },
] as const

export type CustomSectionId = (typeof CUSTOM_SECTIONS)[number]['id']

const customRegistry = createSectionRegistry<
  CustomSectionId,
  CustomSettingsData
>({
  sections: CUSTOM_SECTIONS,
  defaultSection: 'email-templates',
  basePath: '/system-settings/custom',
  urlStyle: 'path',
})

export const CUSTOM_SECTION_IDS = customRegistry.sectionIds
export const CUSTOM_DEFAULT_SECTION = customRegistry.defaultSection
export const getCustomSectionNavItems = customRegistry.getSectionNavItems
export const getCustomSectionContent = customRegistry.getSectionContent
