import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

interface Props {
  settings: Record<string, string>
}

// 生图结果落地保存：应对 CDN（如 CF 橙云）非流式超时导致客户端拿不到
// 已计费的生图响应。开启后成功的生图响应会存盘，用户在"生图记录"页取回。
export function ImageResultSettingsSection({ settings }: Props) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const [enabled, setEnabled] = useState(
    settings.ImageResultEnabled === 'true'
  )
  const [retentionDays, setRetentionDays] = useState(
    settings.ImageResultRetentionDays ?? '7'
  )
  const [maxFileSizeMB, setMaxFileSizeMB] = useState(
    settings.ImageResultMaxFileSizeMB ?? '30'
  )
  const [saving, setSaving] = useState(false)

  const save = async () => {
    const days = parseInt(retentionDays, 10)
    const sizeMB = parseInt(maxFileSizeMB, 10)
    if (Number.isNaN(days) || days < 1 || Number.isNaN(sizeMB) || sizeMB < 1) {
      toast.error(t('Retention days and max file size must be positive integers'))
      return
    }
    setSaving(true)
    try {
      const updates = [
        { key: 'ImageResultEnabled', value: String(enabled) },
        { key: 'ImageResultRetentionDays', value: String(days) },
        { key: 'ImageResultMaxFileSizeMB', value: String(sizeMB) },
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

  return (
    <SettingsSection
      title={t('Image Result Storage')}
      description={t(
        'Save successful image generation responses on the server so users can retrieve them even if the client request timed out (e.g. CDN 120s limit).'
      )}
    >
      <div className='space-y-4'>
        <div className='flex items-center justify-between rounded-lg border p-4'>
          <div>
            <Label className='text-base'>{t('Enable Image Result Storage')}</Label>
            <p className='text-muted-foreground text-sm'>
              {t(
                'Images are decoded/downloaded to local disk; users view them in the Image Results tab of Drawing Logs.'
              )}
            </p>
          </div>
          <Switch checked={enabled} onCheckedChange={setEnabled} />
        </div>

        <div className='grid gap-4 sm:grid-cols-2'>
          <div className='space-y-1'>
            <Label>{t('Retention Days')}</Label>
            <Input
              type='number'
              min='1'
              value={retentionDays}
              onChange={(e) => setRetentionDays(e.target.value)}
            />
            <p className='text-muted-foreground text-xs'>
              {t('Expired results (files and records) are removed by an hourly cleanup task.')}
            </p>
          </div>
          <div className='space-y-1'>
            <Label>{t('Max File Size (MB)')}</Label>
            <Input
              type='number'
              min='1'
              value={maxFileSizeMB}
              onChange={(e) => setMaxFileSizeMB(e.target.value)}
            />
            <p className='text-muted-foreground text-xs'>
              {t('Images larger than this are skipped (logged in backend).')}
            </p>
          </div>
        </div>

        <Button onClick={save} disabled={saving}>
          {saving ? t('Saving...') : t('Save')}
        </Button>
      </div>
    </SettingsSection>
  )
}
