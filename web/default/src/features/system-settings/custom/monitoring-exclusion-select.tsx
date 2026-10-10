import { useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Combobox,
  ComboboxChip,
  ComboboxChips,
  ComboboxChipsInput,
  ComboboxContent,
  ComboboxItem,
  ComboboxList,
  ComboboxValue,
  useComboboxAnchor,
} from '@/components/ui/combobox'
import { useDebounce } from '@/hooks/use-debounce'
import { api } from '@/lib/api'

interface Candidate {
  id: number
  name: string
}
interface Props {
  kind: 'channel' | 'user'
  value: string[]
  onChange: (value: string[]) => void
  id: string
}

export function MonitoringExclusionSelect(props: Props) {
  const { t } = useTranslation()
  const anchor = useComboboxAnchor()
  const [input, setInput] = useState('')
  const keyword = useDebounce(input.trim(), 350)
  const [names, setNames] = useState<Record<string, string>>({})
  const saved = useQuery({
    queryKey: ['monitoring-candidates', props.kind, 'saved'],
    queryFn: async ({ signal }) => {
      const res = await api.get('/api/monitoring/admin/candidates', {
        params: { kind: props.kind, selected: true },
        signal,
        disableDuplicate: true,
        skipErrorHandler: true,
        skipBusinessError: true,
      })
      if (!res.data.success) throw new Error(t('Failed to load'))
      return res.data.data as Candidate[]
    },
    staleTime: 0,
  })
  const search = useQuery({
    queryKey: ['monitoring-candidates', props.kind, 'search', keyword],
    queryFn: async ({ signal }) => {
      const res = await api.get('/api/monitoring/admin/candidates', {
        params: { kind: props.kind, keyword },
        signal,
        disableDuplicate: true,
        skipErrorHandler: true,
        skipBusinessError: true,
      })
      if (!res.data.success) throw new Error(t('Failed to load'))
      return res.data.data as Candidate[]
    },
    enabled: keyword.length > 0 && keyword === input.trim(),
    staleTime: 30_000,
    retry: false,
  })
  useEffect(() => {
    setNames((previous) => {
      const next = { ...previous }
      for (const row of [...(saved.data ?? []), ...(search.data ?? [])]) {
        next[row.id] = row.name
      }
      return next
    })
  }, [saved.data, search.data])

  const pending = input.trim() !== keyword || search.isFetching
  const rows = !pending && input.trim() ? (search.data ?? []) : []
  const label = (id: string) => (names[id] ? `${names[id]} (#${id})` : `#${id}`)

  return (
    <div className='min-w-0'>
      <Combobox
        multiple
        items={rows.map((row) => String(row.id))}
        filter={null}
        value={props.value}
        onValueChange={props.onChange}
        inputValue={input}
        onInputValueChange={setInput}
      >
        <ComboboxChips ref={anchor} className='w-full'>
          <ComboboxValue>
            {(values: string[]) =>
              values.map((id) => (
                <ComboboxChip key={id}>
                  <span className='max-w-48 truncate' title={label(id)}>
                    {label(id)}
                  </span>
                </ComboboxChip>
              ))
            }
          </ComboboxValue>
          <ComboboxChipsInput
            id={props.id}
            aria-label={t('Search by name')}
            placeholder={t('Search by name')}
            maxLength={50}
          />
        </ComboboxChips>
        <ComboboxContent anchor={anchor}>
          <ComboboxList>
            {rows.map((row) => (
              <ComboboxItem key={row.id} value={String(row.id)}>
                <span className='truncate'>
                  {row.name} (#{row.id})
                </span>
              </ComboboxItem>
            ))}
          </ComboboxList>
          {search.isError && (
            <div role='alert' className='text-destructive p-3 text-sm'>
              {t('Failed to load')}
              <Button type='button' variant='ghost' size='sm' onClick={() => void search.refetch()}>{t('Retry')}</Button>
            </div>
          )}
          {!search.isError && rows.length === 0 && (
            <div role='status' className='text-muted-foreground p-3 text-sm'>
              {pending ? t('Loading...') : t('No matching items')}
            </div>
          )}
        </ComboboxContent>
      </Combobox>
      {saved.isError && (
        <div role='alert' className='text-destructive text-sm'>
          {t('Failed to load')}
          <Button
            type='button'
            variant='ghost'
            size='sm'
            onClick={() => {
              void saved.refetch()
            }}
          >
            {t('Retry')}
          </Button>
        </div>
      )}
    </div>
  )
}
