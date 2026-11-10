import { Sparkles } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import { FormNavigationGuard } from '../components/form-navigation-guard'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

export function HeaderStyleSection(props: { value: string }) {
  const { t } = useTranslation()
  const initial = props.value === 'packy' ? 'packy' : 'default'
  const [style, setStyle] = useState(initial)
  const [saved, setSaved] = useState(initial)
  const update = useUpdateOption()

  const save = async () => {
    const result = await update.mutateAsync({
      key: 'HeaderNavStyle',
      value: style,
    })
    if (result.success) setSaved(style)
  }

  return (
    <SettingsSection title={t('Top navigation style')}>
      <FormNavigationGuard when={style !== saved} />
      <form
        onSubmit={(event) => {
          event.preventDefault()
          void save().catch(() => {})
        }}
      >
        <div className='flex flex-wrap items-end gap-3'>
          <div className='space-y-2'>
            <Label htmlFor='header-nav-style'>
              {t('Top navigation style')}
            </Label>
            <Select
              value={style}
              disabled={update.isPending}
              onValueChange={(value: string | null) => {
                if (value === 'default' || value === 'packy') setStyle(value)
              }}
            >
              <SelectTrigger id='header-nav-style' className='w-64 max-w-full'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='default'>{t('Default style')}</SelectItem>
                <SelectItem value='packy'>{t('Packy style')}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <Button type='submit' disabled={update.isPending || style === saved}>
            {t('Save')}
          </Button>
        </div>
        <div
          className='mt-4 overflow-hidden border-y py-4'
          aria-label={t('Preview')}
        >
          <nav
            data-header-style={style}
            data-header-links
            className='flex flex-wrap gap-2 text-sm'
          >
            <a
              href='#header-nav-style'
              aria-current='page'
              onClick={(event) => event.preventDefault()}
            >
              <Sparkles className='header-nav-symbol' aria-hidden='true' />
              {t('Home')}
            </a>
            <a
              href='#header-nav-style'
              onClick={(event) => event.preventDefault()}
            >
              <Sparkles className='header-nav-symbol' aria-hidden='true' />
              {t('Pricing')}
            </a>
            <a
              href='#header-nav-style'
              onClick={(event) => event.preventDefault()}
            >
              <Sparkles className='header-nav-symbol' aria-hidden='true' />
              {t('Documentation')}
            </a>
          </nav>
        </div>
      </form>
    </SettingsSection>
  )
}
