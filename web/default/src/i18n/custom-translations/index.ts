import i18n from 'i18next'
import en from './en'
import zh from './zh'
import zhTW from './zhTW'
import fr from './fr'
import ja from './ja'
import ru from './ru'
import vi from './vi'

// 注意：这里的 key 必须与 i18n/config.ts 的运行时语言代码一致
// （zhCN/zhTW，不是 zh —— load:'currentOnly' 下 'zh' 包永远不会被查询）
const customResources: Record<string, Record<string, string>> = {
  en,
  zhCN: zh,
  zhTW,
  fr,
  ja,
  ru,
  vi,
}

for (const [lang, translations] of Object.entries(customResources)) {
  i18n.addResourceBundle(lang, 'translation', translations, true, false)
}
