import { SettingsPage } from '../components/settings-page'
import type { SiteSettings } from '../types'
import {
  SITE_DEFAULT_SECTION,
  getSiteSectionContent,
} from './section-registry.tsx'

const defaultSiteSettings: SiteSettings = {
  'theme.frontend': 'default',
  Notice: '',
  SystemName: 'New API',
  Logo: '',
  Footer: '',
  About: '',
  HomePageContent: '',
  ServerAddress: '',
  'legal.user_agreement': '',
  'legal.privacy_policy': '',
  'cn_disclaimer.enabled': 'true',
  'cn_disclaimer.title': '',
  'cn_disclaimer.content': '',
  'cn_disclaimer.blocked_countries': '["CN"]',
  'cn_disclaimer.silence_minutes': '60',
  HeaderNavModules: '',
  SidebarModulesAdmin: '',
}

export function SiteSettings() {
  return (
    <SettingsPage
      routePath='/_authenticated/system-settings/site/$section'
      defaultSettings={defaultSiteSettings}
      defaultSection={SITE_DEFAULT_SECTION}
      getSectionContent={getSiteSectionContent}
    />
  )
}
