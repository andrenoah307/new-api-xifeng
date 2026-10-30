/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useMutation, useQuery } from '@tanstack/react-query'
import { Save } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

type CostSettings = {
  mode: string
  ratio: number | null
  sync_status?: string
  updated_at: number
  version: string
}

export function ChannelCostSettings(props: { channelId: number }) {
  const user = useAuthStore((state) => state.auth.user)
  if (user?.role !== 100) return null
  return (
    <CostEditor
      key={`${user.id}:${props.channelId}`}
      channelId={props.channelId}
      userId={user.id}
    />
  )
}

function CostEditor(props: { channelId: number; userId: number }) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState<CostSettings | null>(null)
  const query = useQuery({
    queryKey: ['channel-cost', props.userId, props.channelId],
    queryFn: async () => {
      const response = await api.get<{ success: boolean; data: CostSettings }>(
        `/api/channel-cost/${props.channelId}`
      )
      if (!response.data.success) throw new Error('cost unavailable')
      return response.data.data
    },
    gcTime: 0,
    staleTime: 0,
    retry: false,
  })
  const settings = draft ?? query.data
  const save = useMutation({
    mutationFn: async () => {
      const response = await api.put<{ success: boolean; data: CostSettings }>(
        `/api/channel-cost/${props.channelId}`,
        settings
      )
      if (!response.data.success) throw new Error('cost unavailable')
      return response.data.data
    },
    onSuccess: (data) => {
      setDraft(data)
      toast.success(t('Saved successfully'))
    },
  })
  const prefix = `channel-cost-${props.channelId}`
  if (!settings) {
    return (
      <p className='text-muted-foreground text-sm'>
        {query.isError ? t('Failed to load') : t('Loading...')}
      </p>
    )
  }

  return (
    <section className='grid gap-3 border-t pt-4'>
      <h3 className='text-sm font-medium'>{t('Channel cost')}</h3>
      <Label htmlFor={`${prefix}-mode`}>{t('Cost source')}</Label>
      <NativeSelect
        id={`${prefix}-mode`}
        value={settings.mode || 'disabled'}
        onChange={(event) =>
          setDraft({
            ...settings,
            mode: event.target.value,
          })
        }
      >
        <NativeSelectOption value='disabled'>
          {t('Disabled')}
        </NativeSelectOption>
        <NativeSelectOption value='manual'>{t('Manual')}</NativeSelectOption>
        <NativeSelectOption value='auto'>
          Sub2API - {t('Upstream declared multiplier')}
        </NativeSelectOption>
      </NativeSelect>
      {settings.mode === 'manual' && (
        <>
          <Label htmlFor={`${prefix}-ratio`}>{t('Cost multiplier')}</Label>
          <Input
            id={`${prefix}-ratio`}
            type='number'
            min={0}
            max={10000}
            step='any'
            value={settings.ratio ?? ''}
            onChange={(event) =>
              setDraft({
                ...settings,
                ratio:
                  event.target.value === '' ? null : Number(event.target.value),
              })
            }
          />
        </>
      )}
      {settings.mode === 'auto' && (
        <>
          <p className='text-muted-foreground text-sm'>
            {t('Cost multiplier')}: {settings.ratio ?? '-'}
          </p>
          {settings.sync_status === 'failed' && (
            <p className='text-destructive text-sm'>{t('Failed to load')}</p>
          )}
        </>
      )}
      <Button
        type='button'
        variant='outline'
        className='w-fit'
        disabled={
          save.isPending ||
          (settings.mode === 'manual' &&
            (settings.ratio == null ||
              !Number.isFinite(settings.ratio) ||
              settings.ratio < 0 ||
              settings.ratio > 10000))
        }
        onClick={() => save.mutate()}
      >
        <Save className='size-4' />
        {t('Save cost settings')}
      </Button>
    </section>
  )
}
