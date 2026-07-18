import { RegionRestrictionSection } from '../security/region-restriction-section'
import { CnDisclaimerSection } from '../site/cn-disclaimer-section'
/* eslint-disable react-refresh/only-export-components -- this JSX registry intentionally exports lookup utilities */
import { createSectionRegistry } from '../utils/section-registry'
import { EmailTemplateSettingsSection } from './email-template-settings-section'
import { GroupMonitoringSettingsSection } from './group-monitoring-settings-section'
import { HiddenModelsSection } from './hidden-models-section'
import { TicketSettingsSection } from './ticket-settings-section'

export type CustomSettingsData = Record<string, string>

// 自定义拓展页的 settings 是全量原始 option 字符串（见 index.tsx 的 resolveCustomSettings），
// 没有 SettingsPage 按 defaultSettings 类型做的布尔强转，这里自行解析并保留原页面的默认值
const boolOpt = (v: string | undefined, fallback: boolean): boolean => {
  if (v === undefined || v === '') return fallback
  return v === 'true' || v === '1'
}

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
  {
    id: 'hidden-models',
    titleKey: 'Hidden Models',
    descriptionKey:
      'Models listed here will be hidden from the pricing page but can still be used via API.',
    build: (settings: CustomSettingsData) => (
      <HiddenModelsSection defaultValue={settings.HiddenModels ?? ''} />
    ),
  },
  {
    id: 'cn-disclaimer',
    titleKey: 'Regional access disclaimer',
    descriptionKey:
      'Show a disclaimer modal to users from specific regions until they acknowledge it.',
    build: (settings: CustomSettingsData) => (
      <CnDisclaimerSection
        defaultValues={{
          enabled: settings['cn_disclaimer.enabled'] ?? 'true',
          title: settings['cn_disclaimer.title'] ?? '',
          content: settings['cn_disclaimer.content'] ?? '',
          blockedCountries:
            settings['cn_disclaimer.blocked_countries'] ?? '["CN"]',
          silenceMinutes: settings['cn_disclaimer.silence_minutes'] ?? '60',
        }}
      />
    ),
  },
  {
    id: 'region-restriction',
    titleKey: 'Region Restriction',
    descriptionKey: 'Control model access based on geographic region',
    build: (settings: CustomSettingsData) => (
      <RegionRestrictionSection
        defaultValues={{
          'region_restriction.enabled': boolOpt(
            settings['region_restriction.enabled'],
            false
          ),
          'region_restriction.filter_console': boolOpt(
            settings['region_restriction.filter_console'],
            true
          ),
          'region_restriction.block_relay': boolOpt(
            settings['region_restriction.block_relay'],
            true
          ),
          'region_restriction.xdb_path':
            settings['region_restriction.xdb_path'] || 'data/ip2region.xdb',
          'region_restriction.block_message':
            settings['region_restriction.block_message'] ?? '',
          'region_restriction.console_message':
            settings['region_restriction.console_message'] ?? '',
          'region_restriction.blocked_models':
            settings['region_restriction.blocked_models'] || '{}',
          'region_restriction.blocked_groups':
            settings['region_restriction.blocked_groups'] || '{}',
        }}
      />
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
