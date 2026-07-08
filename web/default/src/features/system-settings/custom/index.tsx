import { SettingsPage } from '../components/settings-page'
import type { SystemOption } from '../types'
import {
  CUSTOM_DEFAULT_SECTION,
  getCustomSectionContent,
  getCustomSectionMeta,
  type CustomSettingsData,
} from './section-registry'

const defaultCustomSettings: CustomSettingsData = {}

// 自定义拓展的三个板块直接读全量 options map（键不固定），
// 与其他设置页枚举 defaultSettings 的方式不同
function resolveCustomSettings(
  settings: CustomSettingsData,
  raw: SystemOption[] | undefined
): CustomSettingsData {
  const next = { ...settings }
  for (const opt of raw ?? []) {
    next[opt.key] = opt.value
  }
  return next
}

export function CustomSettings() {
  return (
    <SettingsPage
      routePath='/_authenticated/system-settings/custom/$section'
      defaultSettings={defaultCustomSettings}
      defaultSection={CUSTOM_DEFAULT_SECTION}
      getSectionContent={getCustomSectionContent}
      getSectionMeta={getCustomSectionMeta}
      loadingMessage='Loading settings...'
      resolveSettings={resolveCustomSettings}
    />
  )
}
