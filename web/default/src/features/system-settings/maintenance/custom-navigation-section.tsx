import { ArrowDown, ArrowUp, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  parseCustomNavigationLinks,
  type CustomNavigationLink,
} from '@/lib/custom-navigation'

import { FormNavigationGuard } from '../components/form-navigation-guard'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

export function CustomNavigationSection(props: { value: string }) {
  const { t } = useTranslation()
  const [links, setLinks] = useState(() =>
    parseCustomNavigationLinks(props.value)
  )
  const [saved, setSaved] = useState(() => JSON.stringify(links))
  const update = useUpdateOption()

  const change = (index: number, patch: Partial<CustomNavigationLink>) => {
    setLinks((previous) =>
      previous.map((link, i) => (i === index ? { ...link, ...patch } : link))
    )
  }
  const move = (index: number, direction: number) => {
    setLinks((previous) => {
      const next = [...previous]
      const target = index + direction
      if (target < 0 || target >= next.length) return previous
      ;[next[index], next[target]] = [next[target], next[index]]
      return next
    })
  }
  const save = async () => {
    const value = JSON.stringify(links)
    const result = await update.mutateAsync({
      key: 'CustomNavigationLinks',
      value,
    })
    if (result.success) setSaved(value)
  }

  return (
    <SettingsSection title={t('Custom navigation links')}>
      <FormNavigationGuard when={JSON.stringify(links) !== saved} />
      <form
        className='space-y-4'
        onSubmit={(event) => {
          event.preventDefault()
          void save().catch(() => {})
        }}
      >
        <fieldset disabled={update.isPending} className='min-w-0 space-y-4'>
          {links.map((link, index) => (
            <div key={link.id} className='space-y-3 border-b pb-4'>
              <div className='grid gap-3 sm:grid-cols-2'>
                <div className='space-y-1'>
                  <Label htmlFor={`${link.id}-title`}>{t('Name')}</Label>
                  <Input
                    id={`${link.id}-title`}
                    required
                    maxLength={80}
                    value={link.title}
                    onChange={(event) =>
                      change(index, { title: event.target.value })
                    }
                  />
                </div>
                <div className='space-y-1'>
                  <Label htmlFor={`${link.id}-url`}>URL</Label>
                  <Input
                    id={`${link.id}-url`}
                    type='url'
                    required
                    maxLength={2048}
                    value={link.url}
                    placeholder='https://example.com'
                    onChange={(event) =>
                      change(index, { url: event.target.value })
                    }
                  />
                </div>
              </div>
              <div className='flex flex-wrap items-center gap-3'>
                <Select
                  value={link.mode}
                  onValueChange={(mode: string | null) => {
                    if (mode === 'external' || mode === 'iframe') {
                      change(index, { mode })
                    }
                  }}
                >
                  <SelectTrigger className='w-48' aria-label={t('Open mode')}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='external'>
                      {t('Open in new tab')}
                    </SelectItem>
                    <SelectItem value='iframe'>{t('Embed in page')}</SelectItem>
                  </SelectContent>
                </Select>
                <Label>
                  <Checkbox
                    checked={link.header}
                    onCheckedChange={(checked) =>
                      change(index, { header: checked === true })
                    }
                  />
                  {t('Top navigation')}
                </Label>
                <Label>
                  <Checkbox
                    checked={link.sidebar}
                    onCheckedChange={(checked) =>
                      change(index, { sidebar: checked === true })
                    }
                  />
                  {t('User sidebar')}
                </Label>
                <Label>
                  <Checkbox
                    checked={link.home === true}
                    onCheckedChange={(checked) =>
                      change(index, { home: checked === true })
                    }
                  />
                  {t('Homepage (after Docs)')}
                </Label>
                <div className='ml-auto flex gap-1'>
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon'
                    title={t('Move up')}
                    aria-label={t('Move up')}
                    disabled={index === 0}
                    onClick={() => move(index, -1)}
                  >
                    <ArrowUp />
                  </Button>
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon'
                    title={t('Move down')}
                    aria-label={t('Move down')}
                    disabled={index === links.length - 1}
                    onClick={() => move(index, 1)}
                  >
                    <ArrowDown />
                  </Button>
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon'
                    title={t('Delete')}
                    aria-label={t('Delete')}
                    onClick={() =>
                      setLinks((previous) =>
                        previous.filter((item) => item.id !== link.id)
                      )
                    }
                  >
                    <Trash2 />
                  </Button>
                </div>
              </div>
            </div>
          ))}
          <div className='flex gap-2'>
            <Button
              type='button'
              variant='outline'
              disabled={links.length >= 30}
              onClick={() =>
                setLinks((previous) => [
                  ...previous,
                  {
                    id: crypto.randomUUID(),
                    title: '',
                    url: '',
                    mode: 'external',
                    header: true,
                    sidebar: true,
                  },
                ])
              }
            >
              <Plus />
              {t('Add')}
            </Button>
            <Button type='submit'>{t('Save')}</Button>
          </div>
        </fieldset>
      </form>
    </SettingsSection>
  )
}
