import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import { RefreshCw } from 'lucide-react'
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
          value: JSON.stringify(monitoringGroups),
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
          value: JSON.stringify(excludeStatusCodes),
        },
        {
          key: PREFIX + 'cache_tokens_separate_groups',
          value: JSON.stringify(cacheSeparateGroups),
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
            onChange={setMonitoringGroups}
            placeholder={t('Select groups...')}
          />
        </div>

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
