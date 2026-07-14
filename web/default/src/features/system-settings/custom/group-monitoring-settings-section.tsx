import { useState, useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import { GripVertical, RefreshCw } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { MultiSelect } from '@/components/multi-select'
import { TagInput } from '@/components/tag-input'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import { api } from '@/lib/api'

const PREFIX = 'group_monitoring_setting.'

function getVal(settings: Record<string, string>, key: string): string {
  return settings[PREFIX + key] ?? ''
}

function parseArr(val: string): string[] {
  if (!val) return []
  try {
    const parsed = JSON.parse(val)
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
}

async function getGroups(): Promise<string[]> {
  const res = await api.get('/api/group/')
  const data = res.data?.data
  if (Array.isArray(data)) return data.map(String)
  if (data && typeof data === 'object') return Object.keys(data)
  return []
}

async function refreshMonitoringData(): Promise<void> {
  await api.post('/api/monitoring/admin/refresh')
}

interface Props {
  settings: Record<string, string>
}

export function GroupMonitoringSettingsSection({ settings }: Props) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const { data: allGroups = [] } = useQuery({
    queryKey: ['groups-list'],
    queryFn: getGroups,
  })

  const [monitoringGroups, setMonitoringGroups] = useState<string[]>(() =>
    parseArr(getVal(settings, 'monitoring_groups'))
  )
  // 显示顺序独立于选择顺序：回显已存的 group_display_order（过滤到当前选中集），
  // 未覆盖的分组按选中顺序补在末尾
  const [displayOrder, setDisplayOrder] = useState<string[]>(() => {
    const selected = parseArr(getVal(settings, 'monitoring_groups'))
    const saved = parseArr(getVal(settings, 'group_display_order')).filter(
      (g) => selected.includes(g)
    )
    return [...saved, ...selected.filter((g) => !saved.includes(g))]
  })
  const dragIndexRef = useRef<number | null>(null)
  const [availabilityPeriod, setAvailabilityPeriod] = useState(
    getVal(settings, 'availability_period_minutes') || '60'
  )
  const [cacheHitPeriod, setCacheHitPeriod] = useState(
    getVal(settings, 'cache_hit_period_minutes') || '60'
  )
  const [aggregationInterval, setAggregationInterval] = useState(
    getVal(settings, 'aggregation_interval_minutes') || '5'
  )
  const [excludeModels, setExcludeModels] = useState<string[]>(() =>
    parseArr(getVal(settings, 'availability_exclude_models'))
  )
  const [cacheExcludeModels, setCacheExcludeModels] = useState<string[]>(() =>
    parseArr(getVal(settings, 'cache_hit_exclude_models'))
  )
  const [excludeKeywords, setExcludeKeywords] = useState<string[]>(() =>
    parseArr(getVal(settings, 'availability_exclude_keywords'))
  )
  const [excludeStatusCodes, setExcludeStatusCodes] = useState<string[]>(() =>
    parseArr(getVal(settings, 'availability_exclude_status_codes'))
  )
  const [cacheSeparateGroups, setCacheSeparateGroups] = useState<string[]>(() =>
    parseArr(getVal(settings, 'cache_tokens_separate_groups'))
  )
  const [frtExcludeThreshold, setFrtExcludeThreshold] = useState(
    getVal(settings, 'frt_exclude_threshold_seconds') || '0'
  )

  const [saving, setSaving] = useState(false)
  const [refreshing, setRefreshing] = useState(false)

  const groupOptions = useMemo(
    () => allGroups.map((g) => ({ label: g, value: g })),
    [allGroups]
  )

  const handleSave = async () => {
    setSaving(true)
    try {
      const updates: Array<{ key: string; value: string }> = [
        {
          key: PREFIX + 'monitoring_groups',
          value: JSON.stringify(monitoringGroups),
        },
        {
          key: PREFIX + 'group_display_order',
          value: JSON.stringify(displayOrder),
        },
        {
          key: PREFIX + 'availability_period_minutes',
          value: availabilityPeriod,
        },
        { key: PREFIX + 'cache_hit_period_minutes', value: cacheHitPeriod },
        {
          key: PREFIX + 'aggregation_interval_minutes',
          value: aggregationInterval,
        },
        {
          key: PREFIX + 'availability_exclude_models',
          value: JSON.stringify(excludeModels),
        },
        {
          key: PREFIX + 'cache_hit_exclude_models',
          value: JSON.stringify(cacheExcludeModels),
        },
        {
          key: PREFIX + 'availability_exclude_keywords',
          value: JSON.stringify(excludeKeywords),
        },
        {
          key: PREFIX + 'availability_exclude_status_codes',
          value: JSON.stringify(excludeStatusCodes.map(s => parseInt(s, 10) || 0).filter(n => n > 0)),
        },
        {
          key: PREFIX + 'cache_tokens_separate_groups',
          value: JSON.stringify(cacheSeparateGroups),
        },
        {
          key: PREFIX + 'frt_exclude_threshold_seconds',
          value: String(parseFloat(frtExcludeThreshold) || 0),
        },
      ]
      for (const u of updates) {
        await updateOption.mutateAsync(u)
      }
      toast.success(t('Config saved'))
    } catch {
      toast.error(t('Operation failed'))
    } finally {
      setSaving(false)
    }
  }

  const handleRefresh = async () => {
    setRefreshing(true)
    try {
      await refreshMonitoringData()
      toast.success(t('Operation successful'))
    } catch {
      toast.error(t('Operation failed'))
    } finally {
      setRefreshing(false)
    }
  }

  return (
    <SettingsSection
      title={t('Group Monitoring Settings')}
      description={t('Configure group monitoring parameters')}
    >
      <div className='space-y-4'>
        <div className='space-y-1'>
          <Label>{t('Monitored Groups')}</Label>
          <MultiSelect
            options={groupOptions}
            selected={monitoringGroups}
            onChange={(next) => {
              setMonitoringGroups(next)
              setDisplayOrder((prev) => [
                ...prev.filter((g) => next.includes(g)),
                ...next.filter((g) => !prev.includes(g)),
              ])
            }}
            placeholder={t('Select groups...')}
          />
        </div>

        {displayOrder.length > 1 && (
          <div className='space-y-1'>
            <Label>{t('Drag to adjust display order')}</Label>
            <div className='flex flex-wrap gap-2'>
              {displayOrder.map((group, idx) => (
                <div
                  key={group}
                  draggable
                  onDragStart={() => {
                    dragIndexRef.current = idx
                  }}
                  onDragOver={(e) => e.preventDefault()}
                  onDrop={(e) => {
                    e.preventDefault()
                    const from = dragIndexRef.current
                    dragIndexRef.current = null
                    if (from == null || from === idx) return
                    setDisplayOrder((prev) => {
                      const next = [...prev]
                      const [moved] = next.splice(from, 1)
                      next.splice(idx, 0, moved)
                      return next
                    })
                  }}
                  className='bg-muted/50 flex cursor-grab items-center gap-1 rounded-md border px-2 py-1 text-sm select-none active:cursor-grabbing'
                >
                  <GripVertical
                    size={13}
                    className='text-muted-foreground/70 shrink-0'
                  />
                  <span className='text-muted-foreground font-mono text-xs'>
                    {idx + 1}.
                  </span>
                  {group}
                </div>
              ))}
            </div>
          </div>
        )}

        <div className='grid gap-4 sm:grid-cols-3'>
          <div className='space-y-1'>
            <Label>{t('Availability Period (min)')}</Label>
            <Input
              type='number'
              value={availabilityPeriod}
              onChange={(e) => setAvailabilityPeriod(e.target.value)}
            />
          </div>
          <div className='space-y-1'>
            <Label>{t('Cache Hit Period (min)')}</Label>
            <Input
              type='number'
              value={cacheHitPeriod}
              onChange={(e) => setCacheHitPeriod(e.target.value)}
            />
          </div>
          <div className='space-y-1'>
            <Label>{t('Aggregation Interval (min)')}</Label>
            <Input
              type='number'
              value={aggregationInterval}
              onChange={(e) => setAggregationInterval(e.target.value)}
            />
          </div>
        </div>

        <div className='space-y-1'>
          <Label>{t('Exclude Models (Availability)')}</Label>
          <TagInput value={excludeModels} onChange={setExcludeModels} />
        </div>

        <div className='space-y-1'>
          <Label>{t('Exclude Models (Cache Hit)')}</Label>
          <TagInput value={cacheExcludeModels} onChange={setCacheExcludeModels} />
        </div>

        <div className='space-y-1'>
          <Label>{t('Exclude Keywords')}</Label>
          <TagInput value={excludeKeywords} onChange={setExcludeKeywords} />
        </div>

        <div className='space-y-1'>
          <Label>{t('Exclude Status Codes')}</Label>
          <TagInput value={excludeStatusCodes} onChange={setExcludeStatusCodes} />
        </div>

        <div className='space-y-1'>
          <Label>{t('Cache Tokens Separate Groups')}</Label>
          <TagInput
            value={cacheSeparateGroups}
            onChange={setCacheSeparateGroups}
          />
        </div>

        <div className='space-y-1'>
          <Label>{t('FRT Exclude Threshold (sec)')}</Label>
          <Input
            type='number'
            min='0'
            step='1'
            className='w-40'
            value={frtExcludeThreshold}
            onChange={(e) => setFrtExcludeThreshold(e.target.value)}
          />
          <p className='text-muted-foreground text-xs'>
            {t(
              'Requests whose first-response time exceeds this are excluded from all monitoring statistics. 0 disables.'
            )}
          </p>
        </div>

        <div className='flex gap-2'>
          <Button onClick={handleSave} disabled={saving}>
            {saving ? t('Saving...') : t('Save')}
          </Button>
          <Button
            variant='outline'
            onClick={handleRefresh}
            disabled={refreshing}
          >
            <RefreshCw className='mr-1 h-3.5 w-3.5' />
            {t('Refresh now')}
          </Button>
        </div>
      </div>
    </SettingsSection>
  )
}
