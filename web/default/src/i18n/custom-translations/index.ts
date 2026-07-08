import i18n from 'i18next'
import en from './en'
import zh from './zh'
import fr from './fr'
import ja from './ja'
import ru from './ru'
import vi from './vi'

const customResources: Record<string, Record<string, string>> = {
  en,
  zh,
  fr,
  ja,
  ru,
  vi,
}

for (const [lang, translations] of Object.entries(customResources)) {
  i18n.addResourceBundle(lang, 'translation', translations, true, false)
}
