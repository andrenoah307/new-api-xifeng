import { RateLimitSection } from '../request-limits/rate-limit-section'
import { SensitiveWordsSection } from '../request-limits/sensitive-words-section'
import { SSRFSection } from '../request-limits/ssrf-section'
import type { SecuritySettings } from '../types'
import { createSectionRegistry } from '../utils/section-registry'
import { GroupModelBlacklistSection } from './group-model-blacklist-section'
import { RegionRestrictionSection } from './region-restriction-section'

const SECURITY_SECTIONS = [
  {
    id: 'rate-limit',
    titleKey: 'Rate Limiting',
    descriptionKey: 'Configure model request rate limiting',
    build: (settings: SecuritySettings) => (
      <RateLimitSection
        defaultValues={{
          ModelRequestRateLimitEnabled: settings.ModelRequestRateLimitEnabled,
          ModelRequestRateLimitCount: settings.ModelRequestRateLimitCount,
          ModelRequestRateLimitSuccessCount:
            settings.ModelRequestRateLimitSuccessCount,
          ModelRequestRateLimitDurationMinutes:
            settings.ModelRequestRateLimitDurationMinutes,
          ModelRequestRateLimitGroup: settings.ModelRequestRateLimitGroup,
        }}
      />
    ),
  },
  {
    id: 'sensitive-words',
    titleKey: 'Sensitive Words',
    descriptionKey: 'Configure sensitive word filtering',
    build: (settings: SecuritySettings) => (
      <SensitiveWordsSection
        defaultValues={{
          CheckSensitiveEnabled: settings.CheckSensitiveEnabled,
          CheckSensitiveOnPromptEnabled: settings.CheckSensitiveOnPromptEnabled,
          SensitiveWords: settings.SensitiveWords,
        }}
      />
    ),
  },
  {
    id: 'ssrf',
    titleKey: 'SSRF Protection',
    descriptionKey: 'Configure SSRF (Server-Side Request Forgery) protection',
    build: (settings: SecuritySettings) => (
      <SSRFSection
        defaultValues={{
          'fetch_setting.enable_ssrf_protection':
            settings['fetch_setting.enable_ssrf_protection'],
          'fetch_setting.allow_private_ip':
            settings['fetch_setting.allow_private_ip'],
          'fetch_setting.domain_filter_mode':
            settings['fetch_setting.domain_filter_mode'],
          'fetch_setting.ip_filter_mode':
            settings['fetch_setting.ip_filter_mode'],
          'fetch_setting.domain_list': settings['fetch_setting.domain_list'],
          'fetch_setting.ip_list': settings['fetch_setting.ip_list'],
          'fetch_setting.allowed_ports':
            settings['fetch_setting.allowed_ports'],
          'fetch_setting.apply_ip_filter_for_domain':
            settings['fetch_setting.apply_ip_filter_for_domain'],
        }}
      />
    ),
  },
  {
    id: 'region-restriction',
    titleKey: 'Region Restriction',
    descriptionKey: 'Control model access based on geographic region',
    build: (settings: SecuritySettings) => (
      <RegionRestrictionSection
        defaultValues={{
          'region_restriction.enabled': settings['region_restriction.enabled'],
          'region_restriction.filter_console':
            settings['region_restriction.filter_console'],
          'region_restriction.block_relay':
            settings['region_restriction.block_relay'],
          'region_restriction.xdb_path':
            settings['region_restriction.xdb_path'],
          'region_restriction.block_message':
            settings['region_restriction.block_message'],
          'region_restriction.blocked_models':
            settings['region_restriction.blocked_models'],
        }}
      />
    ),
  },
  {
    id: 'group-model-blacklist',
    titleKey: 'Group Model Blacklist',
    descriptionKey: 'Control model access based on user group',
    build: (settings: SecuritySettings) => (
      <GroupModelBlacklistSection
        defaultValues={{
          'group_model_blacklist.enabled':
            settings['group_model_blacklist.enabled'],
          'group_model_blacklist.filter_console':
            settings['group_model_blacklist.filter_console'],
          'group_model_blacklist.block_relay':
            settings['group_model_blacklist.block_relay'],
          'group_model_blacklist.block_message':
            settings['group_model_blacklist.block_message'],
          'group_model_blacklist.blocked_models':
            settings['group_model_blacklist.blocked_models'],
        }}
      />
    ),
  },
] as const

export type SecuritySectionId = (typeof SECURITY_SECTIONS)[number]['id']

const securityRegistry = createSectionRegistry<
  SecuritySectionId,
  SecuritySettings
>({
  sections: SECURITY_SECTIONS,
  defaultSection: 'rate-limit',
  basePath: '/system-settings/security',
  urlStyle: 'path',
})

export const SECURITY_SECTION_IDS = securityRegistry.sectionIds
export const SECURITY_DEFAULT_SECTION = securityRegistry.defaultSection
export const getSecuritySectionNavItems = securityRegistry.getSectionNavItems
export const getSecuritySectionContent = securityRegistry.getSectionContent
