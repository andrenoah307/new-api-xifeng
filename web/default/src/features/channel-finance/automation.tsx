import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Eye, Mail, Plus, RefreshCw, Save, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useFieldArray, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { api } from '@/lib/api'

import { CostConfirmation } from './cost-confirmation'

type RevenueField = {
  key: string
  metric: string
  period: string
  days: number
  channel_id: number
}
type Config = {
  enabled: boolean
  time: string
  periods: string[]
  mail_enabled: boolean
  mail_period: string
  recipients: string[]
  subject: string
  body: string
  fields: RevenueField[]
  fingerprint: string
}
type Run = { id: string; scheduled_at: number; status: string; error: string }
type Response = {
  config: Config
  revision: number
  next_at: number
  runs: Run[]
}
type Editor = Omit<Config, 'recipients'> & { recipientsText: string }
const periods = [
  ['day', 'Previous day'],
  ['week', 'Previous week'],
  ['month', 'Previous month'],
  ['quarter', 'Previous quarter'],
  ['year', 'Previous year'],
] as const
const metrics = [
  ['consumption', 'Consumption'],
  ['cost', 'Cost_finance'],
  ['profit', 'Profit'],
  ['profit_margin', 'Profit margin'],
] as const
const cadences = [
  ['day', 'Daily'],
  ['week', 'Weekly'],
  ['month', 'Monthly'],
  ['quarter', 'Quarterly'],
  ['year', 'Yearly'],
] as const

function date(value: number) {
  return new Date(value * 1000).toLocaleString('zh-CN', {
    timeZone: 'Asia/Shanghai',
    hour12: false,
  })
}

export function FinanceAutomation(props: { userId: number }) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['revenue-automation', props.userId],
    queryFn: async ({ signal }) => {
      const result = await api.get('/api/channel-finance/automation', {
        signal,
        disableDuplicate: true,
      })
      if (!result.data.success) throw new Error(result.data.message)
      return result.data.data as Response
    },
    refetchInterval: 30000,
    gcTime: 0,
  })
  if (query.error) return <p role='alert'>{query.error.message}</p>
  if (!query.data) return <p>{t('Loading...')}</p>
  return (
    <AutomationEditor
      key={query.data.revision}
      data={query.data}
      userId={props.userId}
    />
  )
}

function AutomationEditor(props: { data: Response; userId: number }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const form = useForm<Editor>({
    defaultValues: {
      ...props.data.config,
      recipientsText: props.data.config.recipients.join('\n'),
    },
  })
  const fields = useFieldArray({ control: form.control, name: 'fields' })
  const values = form.watch()
  const [confirmation, setConfirmation] = useState<Config | null>(null)
  const [preview, setPreview] = useState<{
    subject: string
    body: string
  } | null>(null)
  const [detail, setDetail] = useState<{
    run: Run
    data: {
      values?: Record<string, string>
      deliveries?: { recipient: string; status: string }[]
      job_id?: string
    }
  } | null>(null)
  const [message, setMessage] = useState('')
  const save = useMutation({
    mutationFn: async (config: Config) => {
      const result = await api.put('/api/channel-finance/automation', {
        revision: props.data.revision,
        config,
      })
      if (!result.data.success) throw new Error(result.data.message)
    },
    onSuccess: () => {
      setConfirmation(null)
      setMessage(t('Saved'))
      void client.invalidateQueries({
        queryKey: ['revenue-automation', props.userId],
      })
    },
  })
  const mail = useMutation({
    mutationFn: async (action: string) => {
      const result = await api.post(
        `/api/channel-finance/automation/mail/${action}`,
        configuration(form.getValues())
      )
      if (!result.data.success) throw new Error(result.data.message)
      return { ...result.data.data, action }
    },
    onSuccess: (data) => {
      setPreview(data)
      setMessage(data.action === 'test' ? t('Test email sent') : '')
    },
  })
  const inspect = useMutation({
    mutationFn: async (id: string) => {
      const result = await api.get(`/api/channel-finance/automation/runs/${id}`)
      if (!result.data.success) throw new Error(result.data.message)
      return result.data.data
    },
    onSuccess: setDetail,
  })
  const control = useMutation({
    mutationFn: async (action: string) => {
      const result = await api.post(
        `/api/channel-finance/automation/runs/${detail?.run.id}/${action}?confirm_duplicate_risk=true`
      )
      if (!result.data.success) throw new Error(result.data.message)
    },
    onSuccess: () => {
      setDetail(null)
      void client.invalidateQueries({
        queryKey: ['revenue-automation', props.userId],
      })
    },
  })
  const busy = save.isPending || mail.isPending || control.isPending
  return (
    <div className='space-y-6 py-4'>
      <form
        className='space-y-6'
        onSubmit={form.handleSubmit((input) => {
          const config = configuration(input)
          setMessage('')
          if (config.enabled) setConfirmation(config)
          else save.mutate(config)
        })}
      >
        <fieldset className='space-y-4 border-b pb-5' disabled={busy}>
          <legend className='mb-3 text-base font-medium'>
            {t('Scheduled summaries')}
          </legend>
          <label className='flex items-center gap-2'>
            <input type='checkbox' {...form.register('enabled')} />
            {t('Enabled')}
          </label>
          <label className='block max-w-64 space-y-2'>
            <span>{t('Execution time (UTC+8)')}</span>
            <Input type='time' required {...form.register('time')} />
          </label>
          <div className='flex flex-wrap gap-4'>
            {periods.map(([key, label]) => (
              <label key={key} className='flex items-center gap-2'>
                <input
                  type='checkbox'
                  value={key}
                  {...form.register('periods')}
                />
                {t(label)}
              </label>
            ))}
          </div>
          <p className='text-sm text-muted-foreground'>
            {t('Next run')}:{' '}
            {values.enabled && props.data.next_at
              ? date(props.data.next_at)
              : '-'}
          </p>
        </fieldset>
        <fieldset className='space-y-4 border-b pb-5' disabled={busy}>
          <legend className='mb-3 text-base font-medium'>
            {t('Email notifications')}
          </legend>
          <label className='flex items-center gap-2'>
            <input type='checkbox' {...form.register('mail_enabled')} />
            {t('Enabled')}
          </label>
          <label className='block space-y-2'>
            <span>{t('Mail cadence')}</span>
            <NativeSelect {...form.register('mail_period')}>
              {cadences.map(([key, label]) => (
                <option key={key} value={key}>
                  {t(label)}
                </option>
              ))}
            </NativeSelect>
          </label>
          <label className='block space-y-2'>
            <span>{t('Recipients')}</span>
            <Textarea rows={3} {...form.register('recipientsText')} />
          </label>
          <p className='text-sm text-muted-foreground'>
            {t('Known-cost records only')}
          </p>
          {fields.fields.map((field, index) => (
            <div
              key={field.id}
              className='grid items-end gap-3 border-b pb-3 sm:grid-cols-2 lg:grid-cols-6'
            >
              <label className='space-y-2'>
                <span>{t('Field key')}</span>
                <Input
                  required
                  pattern='[\p{L}][\p{L}\p{N}_]{0,47}'
                  {...form.register(`fields.${index}.key`)}
                />
              </label>
              <label className='space-y-2'>
                <span>{t('Metric')}</span>
                <NativeSelect
                  className='w-full'
                  {...form.register(`fields.${index}.metric`)}
                >
                  {metrics.map(([key, label]) => (
                    <option key={key} value={key}>
                      {t(label)}
                    </option>
                  ))}
                </NativeSelect>
              </label>
              <label className='space-y-2'>
                <span>{t('Time window')}</span>
                <NativeSelect
                  className='w-full'
                  {...form.register(`fields.${index}.period`)}
                >
                  {periods.map(([key, label]) => (
                    <option key={key} value={key}>
                      {t(label)}
                    </option>
                  ))}
                  <option value='days'>{t('Previous complete days')}</option>
                </NativeSelect>
              </label>
              <label className='space-y-2'>
                <span>{t('Days')}</span>
                <Input
                  type='number'
                  min={1}
                  max={366}
                  disabled={values.fields[index]?.period !== 'days'}
                  {...form.register(`fields.${index}.days`, {
                    valueAsNumber: true,
                  })}
                />
              </label>
              <label className='space-y-2'>
                <span>{t('Channel ID (0 = all)')}</span>
                <Input
                  type='number'
                  min={0}
                  {...form.register(`fields.${index}.channel_id`, {
                    valueAsNumber: true,
                  })}
                />
              </label>
              <Button
                type='button'
                variant='ghost'
                size='icon'
                title={t('Delete')}
                aria-label={t('Delete')}
                onClick={() => fields.remove(index)}
              >
                <Trash2 />
              </Button>
            </div>
          ))}
          <Button
            type='button'
            variant='outline'
            disabled={fields.fields.length >= 20}
            onClick={() =>
              fields.append({
                key: `field_${fields.fields.length + 1}`,
                metric: 'profit',
                period: 'day',
                days: 1,
                channel_id: 0,
              })
            }
          >
            <Plus />
            {t('Add field')}
          </Button>
          <label className='block space-y-2'>
            <span>{t('Subject')}</span>
            <Input {...form.register('subject')} maxLength={200} />
          </label>
          <label className='block space-y-2'>
            <span>{t('Body')}</span>
            <Textarea rows={8} {...form.register('body')} maxLength={32768} />
          </label>
          <label className='block space-y-2'>
            <span>{t('Insert placeholder')}</span>
            <NativeSelect
              value=''
              onChange={(event) =>
                form.setValue(
                  'body',
                  `${form.getValues('body')}{{${event.target.value}}}`,
                  { shouldDirty: true }
                )
              }
            >
              <option value='' disabled>
                {t('Insert placeholder')}
              </option>
              {[
                'system_name',
                'scheduled_at',
                'generated_at',
                'run_id',
                'summary_table',
                ...values.fields.flatMap((field) =>
                  ['value', 'start', 'end', 'unit', 'coverage'].map(
                    (part) => `fields.${field.key}.${part}`
                  )
                ),
              ].map((key) => (
                <option key={key} value={key}>{`{{${key}}}`}</option>
              ))}
            </NativeSelect>
          </label>
          <div className='flex flex-wrap gap-2'>
            <Button
              type='button'
              variant='outline'
              onClick={() => mail.mutate('preview')}
            >
              <Eye />
              {t('Preview sample')}
            </Button>
            <Button
              type='button'
              variant='outline'
              onClick={() => mail.mutate('test')}
            >
              <Mail />
              {t('Send test to myself')}
            </Button>
          </div>
        </fieldset>
        <Button disabled={busy} type='submit'>
          <Save />
          {t('Save')}
        </Button>
        {(save.error || mail.error || inspect.error || control.error) && (
          <p role='alert' className='text-destructive'>
            {
              (save.error || mail.error || inspect.error || control.error)
                ?.message
            }
          </p>
        )}
        {message && <p role='status'>{message}</p>}
      </form>
      {preview && (
        <section className='space-y-2'>
          <h3 className='font-medium'>
            {t('Preview sample')}: {preview.subject}
          </h3>
          <iframe
            title={t('Preview sample')}
            sandbox=''
            referrerPolicy='no-referrer'
            className='h-80 w-full border'
            srcDoc={`<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; form-action 'none'; base-uri 'none'">${preview.body}`}
          />
        </section>
      )}
      <section className='space-y-3'>
        <div className='flex items-center justify-between'>
          <h3 className='font-medium'>{t('Execution history')}</h3>
          <Button
            type='button'
            variant='ghost'
            size='icon'
            title={t('Refresh')}
            aria-label={t('Refresh')}
            onClick={() =>
              void client.invalidateQueries({
                queryKey: ['revenue-automation', props.userId],
              })
            }
          >
            <RefreshCw />
          </Button>
        </div>
        <div className='overflow-x-auto'>
          <table className='w-full text-sm'>
            <thead>
              <tr>
                <th className='p-2 text-left'>{t('Scheduled time')}</th>
                <th className='p-2 text-left'>{t('Status')}</th>
                <th className='p-2 text-left'>{t('Details')}</th>
              </tr>
            </thead>
            <tbody>
              {props.data.runs.map((run) => (
                <tr key={run.id} className='border-t'>
                  <td className='whitespace-nowrap p-2'>
                    {date(run.scheduled_at)}
                  </td>
                  <td className='p-2'>{t(`Revenue status ${run.status}`)}</td>
                  <td className='p-2'>
                    <Button
                      type='button'
                      variant='ghost'
                      size='icon'
                      aria-label={t('Details')}
                      title={t('Details')}
                      onClick={() => inspect.mutate(run.id)}
                    >
                      <Eye />
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {detail && (
          <div className='space-y-2 break-words text-sm'>
            <p>{detail.run.id}</p>
            {['failed', 'unknown'].includes(detail.run.status) && (
              <div className='flex flex-wrap gap-2'>
                <Button
                  disabled={busy}
                  onClick={() => {
                    if (
                      window.confirm(
                        t('Retry may resend uncertain email. Continue?')
                      )
                    ) {
                      control.mutate('retry')
                    }
                  }}
                >
                  {t('Retry')}
                </Button>
                <Button
                  disabled={busy}
                  onClick={() => {
                    if (window.confirm(t('Dismiss this automation run?'))) {
                      control.mutate('dismiss')
                    }
                  }}
                >
                  {t('Dismiss')}
                </Button>
              </div>
            )}
            {detail.run.error && <p role='alert'>{detail.run.error}</p>}
            {detail.data.job_id && (
              <p>
                {t('Summary task')}: {detail.data.job_id}
              </p>
            )}
            {detail.data.deliveries?.map((item) => (
              <p key={item.recipient}>
                {item.recipient}: {t(`Revenue status ${item.status}`)}
              </p>
            ))}
            <dl className='grid gap-2 sm:grid-cols-2'>
              {Object.entries(detail.data.values ?? {})
                .filter(([key]) => key !== 'summary_table')
                .map(([key, value]) => (
                  <div key={key}>
                    <dt className='text-muted-foreground'>{key}</dt>
                    <dd>{value}</dd>
                  </div>
                ))}
            </dl>
          </div>
        )}
      </section>
      {confirmation && (
        <CostConfirmation
          confirmLabel='Confirm and enable'
          request={{
            request_key: 'automation',
            start: 1,
            end: 2,
            channel_id: 0,
            fallback_mode: 'billed_basis',
          }}
          userId={props.userId}
          busy={save.isPending}
          error={save.error?.message ?? ''}
          onClose={() => setConfirmation(null)}
          onConfirm={(fingerprint) =>
            save.mutate({ ...confirmation, fingerprint })
          }
        />
      )}
    </div>
  )
}

function configuration(input: Editor): Config {
  const { recipientsText, ...config } = input
  return {
    ...config,
    recipients: recipientsText
      .split(/[\n;,]+/)
      .map((value) => value.trim())
      .filter(Boolean),
    fields: config.fields.map((field) => ({ ...field, days: field.days || 1 })),
  }
}
