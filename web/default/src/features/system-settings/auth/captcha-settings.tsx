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
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { api } from '@/lib/api'

import {
  captchaFields,
  captchaScenes,
  captchaSecretFields,
  type CaptchaConfig,
  type CaptchaSettingsData,
} from '../../../../../shared/captcha-settings'
import { SettingsPageFormActions } from '../components/settings-page-context'

export function CaptchaSettings() {
  const { t } = useTranslation()
  const [snapshot, setSnapshot] = useState<CaptchaSettingsData | null>(null)
  const [error, setError] = useState(false)
  const [cleared, setCleared] = useState<string[]>([])
  const form = useForm<CaptchaConfig>()
  const {
    reset,
    register,
    handleSubmit,
    formState: { isSubmitting },
  } = form

  useEffect(() => {
    let active = true
    api
      .get('/api/captcha/settings')
      .then(({ data }) => {
        if (!active) return
        if (!data.success) {
          setError(true)
          return
        }
        setSnapshot(data.data)
        reset(data.data.config)
      })
      .catch(() => {
        if (active) setError(true)
      })
    return () => {
      active = false
    }
  }, [reset])

  const save = handleSubmit(async (config) => {
    if (!snapshot) return
    try {
      const { data } = await api.put('/api/captcha/settings', {
        config,
        revision: snapshot.revision,
        clear_secrets: cleared,
      })
      if (!data.success) return
      setSnapshot(data.data)
      reset(data.data.config)
      setCleared([])
      toast.success(t('Saved successfully'))
    } catch {
      /* The API interceptor displays the error. */
    }
  })

  if (error) return <p role='alert'>{t('Failed to load settings')}</p>
  if (!snapshot) return <p role='status'>{t('Loading...')}</p>
  return (
    <form onSubmit={save} autoComplete='off' className='space-y-6'>
      <SettingsPageFormActions onSave={save} isSaving={isSubmitting} />
      <fieldset disabled={isSubmitting} className='min-w-0 space-y-6'>
        <label className='flex items-center justify-between gap-4 text-sm font-medium'>
          {t('Enable captcha protection')}
          <Switch
            checked={form.watch('enabled')}
            onCheckedChange={(value) =>
              form.setValue('enabled', value, { shouldDirty: true })
            }
          />
        </label>
        {['turnstile', 'tencent'].map((provider) => (
          <section key={provider} className='space-y-4 border-t pt-4'>
            <h3 className='text-sm font-semibold'>
              {provider === 'turnstile'
                ? 'Cloudflare Turnstile'
                : t('Tencent Cloud Captcha')}
            </h3>
            <div className='grid gap-4 md:grid-cols-2'>
              {Object.entries(captchaFields)
                .filter(([key]) => key.startsWith(provider))
                .map(([key, label]) => {
                  const secret = captchaSecretFields.includes(key)
                  return (
                    <div key={key} className='min-w-0 space-y-2'>
                      <label
                        htmlFor={`captcha-${key}`}
                        className='text-sm font-medium'
                      >
                        {t(label)}
                      </label>
                      <Input
                        id={`captcha-${key}`}
                        type={secret ? 'password' : 'text'}
                        autoComplete={secret ? 'new-password' : 'off'}
                        maxLength={2048}
                        disabled={cleared.includes(key)}
                        placeholder={
                          secret && snapshot.configured[key]
                            ? t('Configured; leave blank to keep')
                            : ''
                        }
                        {...register(
                          key as keyof Omit<
                            CaptchaConfig,
                            'scenes' | 'enabled' | 'tencent_app_id_auth'
                          >
                        )}
                      />
                      {secret && (
                        <label className='flex items-center gap-2 text-sm'>
                          <input
                            type='checkbox'
                            checked={cleared.includes(key)}
                            onChange={(event) =>
                              setCleared((old) =>
                                event.target.checked
                                  ? [...old, key]
                                  : old.filter((item) => item !== key)
                              )
                            }
                          />
                          {t('Clear saved secret')}
                        </label>
                      )}
                    </div>
                  )
                })}
            </div>
            {provider === 'tencent' && (
              <label className='flex items-center justify-between gap-4 text-sm'>
                {t('CaptchaAppId authentication')}
                <Switch
                  checked={form.watch('tencent_app_id_auth')}
                  onCheckedChange={(value) =>
                    form.setValue('tencent_app_id_auth', value, {
                      shouldDirty: true,
                    })
                  }
                />
              </label>
            )}
          </section>
        ))}
        <section className='space-y-3 border-t pt-4'>
          <h3 className='text-sm font-semibold'>{t('Protected actions')}</h3>
          {Object.entries(captchaScenes).map(([scene, label]) => (
            <label
              key={scene}
              className='grid grid-cols-[minmax(0,1fr)_160px] items-center gap-3 text-sm'
            >
              <span>{t(label)}</span>
              <select
                className='bg-background h-9 min-w-0 rounded-md border px-2'
                {...register(`scenes.${scene}`)}
                defaultValue={snapshot.config.scenes[scene] || 'off'}
              >
                <option value='off'>{t('Disabled')}</option>
                <option value='turnstile'>Turnstile</option>
                <option value='tencent'>Tencent</option>
              </select>
            </label>
          ))}
        </section>
        <Button type='submit'>{t('Save')}</Button>
      </fieldset>
    </form>
  )
}
