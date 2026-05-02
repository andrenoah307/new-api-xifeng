import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import { api } from '@/lib/api'

interface StaffMember {
  id: number
  username: string
  display_name: string
  role: number
  email: string
}

interface AssignRule {
  strategy: string
  users: number[]
}

interface AssignConfig {
  enabled: boolean
  fallback: string
  rules: Record<string, AssignRule>
}

async function getStaffList(): Promise<StaffMember[]> {
  const res = await api.get('/api/ticket/admin/staff')
  return res.data?.data ?? []
}

function parseJSON<T>(val: string | undefined, fallback: T): T {
  if (!val) return fallback
  try {
    return JSON.parse(val) as T
  } catch {
    return fallback
  }
}

const TICKET_TYPES = ['general', 'refund', 'invoice'] as const
const STRATEGIES = ['round_robin', 'least_loaded', 'random', 'manual'] as const
const STORAGE_BACKENDS = ['local', 'oss', 's3', 'cos'] as const

interface Props {
  settings: Record<string, string>
}

export function TicketSettingsSection({ settings }: Props) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const { data: staffList = [] } = useQuery({
    queryKey: ['ticket-staff'],
    queryFn: getStaffList,
  })

  const defaultAssignConfig: AssignConfig = {
    enabled: false,
    fallback: 'none',
    rules: {
      general: { strategy: 'round_robin', users: [] },
      refund: { strategy: 'round_robin', users: [] },
      invoice: { strategy: 'round_robin', users: [] },
    },
  }

  const [assignConfig, setAssignConfig] = useState<AssignConfig>(() =>
    parseJSON(settings.TicketAssignConfig, defaultAssignConfig)
  )
  const [notifyEnabled, setNotifyEnabled] = useState(
    settings.TicketNotifyEnabled === 'true'
  )
  const [adminEmail, setAdminEmail] = useState(
    settings.TicketAdminEmail ?? ''
  )
  const [attachEnabled, setAttachEnabled] = useState(
    settings.TicketAttachmentEnabled === 'true'
  )
  const [maxSize, setMaxSize] = useState(
    settings.TicketAttachmentMaxSize ?? '52428800'
  )
  const [maxCount, setMaxCount] = useState(
    settings.TicketAttachmentMaxCount ?? '5'
  )
  const [allowedExts, setAllowedExts] = useState(
    settings.TicketAttachmentAllowedExts ?? ''
  )
  const [allowedMimes, setAllowedMimes] = useState(
    settings.TicketAttachmentAllowedMimes ?? ''
  )
  const [storage, setStorage] = useState(
    settings.TicketAttachmentStorage ?? 'local'
  )
  const [localPath, setLocalPath] = useState(
    settings.TicketAttachmentLocalPath ?? ''
  )
  const [signedUrlTTL, setSignedUrlTTL] = useState(
    settings.TicketAttachmentSignedURLTTL ?? '3600'
  )
  const [cloudConfig, setCloudConfig] = useState<Record<string, string>>(() => {
    const cfg: Record<string, string> = {}
    for (const key of Object.keys(settings)) {
      if (
        key.startsWith('TicketAttachment') &&
        !['TicketAttachmentEnabled', 'TicketAttachmentMaxSize', 'TicketAttachmentMaxCount', 'TicketAttachmentAllowedExts', 'TicketAttachmentAllowedMimes', 'TicketAttachmentStorage', 'TicketAttachmentLocalPath', 'TicketAttachmentSignedURLTTL'].includes(key)
      ) {
        cfg[key] = settings[key]
      }
    }
    return cfg
  })

  const [saving, setSaving] = useState(false)

  const staffMap = useMemo(
    () => new Map(staffList.map((s) => [s.id, s])),
    [staffList]
  )

  const updateAssignRule = (
    type: string,
    field: keyof AssignRule,
    value: unknown
  ) => {
    setAssignConfig((prev) => ({
      ...prev,
      rules: {
        ...prev.rules,
        [type]: { ...prev.rules[type], [field]: value },
      },
    }))
  }

  const toggleStaffUser = (type: string, userId: number) => {
    setAssignConfig((prev) => {
      const rule = prev.rules[type]
      const users = rule.users.includes(userId)
        ? rule.users.filter((id) => id !== userId)
        : [...rule.users, userId]
      return {
        ...prev,
        rules: { ...prev.rules, [type]: { ...rule, users } },
      }
    })
  }

  const saveAssignment = async () => {
    setSaving(true)
    try {
      await updateOption.mutateAsync({
        key: 'TicketAssignConfig',
        value: JSON.stringify(assignConfig),
      })
      toast.success(t('Config saved'))
    } catch {
      toast.error(t('Operation failed'))
    } finally {
      setSaving(false)
    }
  }

  const saveNotification = async () => {
    setSaving(true)
    try {
      await updateOption.mutateAsync({
        key: 'TicketNotifyEnabled',
        value: String(notifyEnabled),
      })
      await updateOption.mutateAsync({
        key: 'TicketAdminEmail',
        value: adminEmail,
      })
      toast.success(t('Config saved'))
    } catch {
      toast.error(t('Operation failed'))
    } finally {
      setSaving(false)
    }
  }

  const saveAttachment = async () => {
    setSaving(true)
    try {
      const updates: Array<{ key: string; value: string }> = [
        { key: 'TicketAttachmentEnabled', value: String(attachEnabled) },
        { key: 'TicketAttachmentMaxSize', value: maxSize },
        { key: 'TicketAttachmentMaxCount', value: maxCount },
        { key: 'TicketAttachmentAllowedExts', value: allowedExts },
        { key: 'TicketAttachmentAllowedMimes', value: allowedMimes },
        { key: 'TicketAttachmentStorage', value: storage },
        { key: 'TicketAttachmentLocalPath', value: localPath },
        { key: 'TicketAttachmentSignedURLTTL', value: signedUrlTTL },
      ]
      for (const [k, v] of Object.entries(cloudConfig)) {
        updates.push({ key: k, value: v })
      }
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

  const cloudFields = getCloudFields(storage)

  return (
    <SettingsSection
      title={t('Ticket Settings')}
      description={t('Configure ticket system settings')}
    >
      <Tabs defaultValue='assignment'>
        <TabsList>
          <TabsTrigger value='assignment'>
            {t('Assignment Rules')}
          </TabsTrigger>
          <TabsTrigger value='notification'>
            {t('Email Notification')}
          </TabsTrigger>
          <TabsTrigger value='attachment'>
            {t('Attachment Settings')}
          </TabsTrigger>
        </TabsList>

        <TabsContent value='assignment' className='space-y-4 pt-4'>
          <div className='flex items-center justify-between rounded-lg border p-4'>
            <div>
              <Label className='text-base'>{t('Auto Assign')}</Label>
              <p className='text-muted-foreground text-sm'>
                {t('Automatically assign tickets to staff members')}
              </p>
            </div>
            <Switch
              checked={assignConfig.enabled}
              onCheckedChange={(v) =>
                setAssignConfig((prev) => ({ ...prev, enabled: v }))
              }
            />
          </div>

          <div className='space-y-1'>
            <Label>{t('Fallback Strategy')}</Label>
            <Select
              value={assignConfig.fallback}
              onValueChange={(v) =>
                setAssignConfig((prev) => ({ ...prev, fallback: v }))
              }
            >
              <SelectTrigger className='w-48'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='none'>{t('None')}</SelectItem>
                <SelectItem value='general_group'>
                  {t('General Group')}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>

          {TICKET_TYPES.map((type) => {
            const rule = assignConfig.rules[type] ?? {
              strategy: 'round_robin',
              users: [],
            }
            return (
              <div key={type} className='space-y-2 rounded-lg border p-4'>
                <Label className='capitalize'>{t(type)}</Label>
                <div className='flex items-center gap-3'>
                  <Select
                    value={rule.strategy}
                    onValueChange={(v) =>
                      updateAssignRule(type, 'strategy', v)
                    }
                  >
                    <SelectTrigger className='w-40'>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {STRATEGIES.map((s) => (
                        <SelectItem key={s} value={s}>
                          {t(s)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className='flex flex-wrap gap-2'>
                  {staffList.map((staff) => (
                    <Button
                      key={staff.id}
                      size='sm'
                      variant={
                        rule.users.includes(staff.id) ? 'default' : 'outline'
                      }
                      onClick={() => toggleStaffUser(type, staff.id)}
                    >
                      {staff.display_name || staff.username}
                    </Button>
                  ))}
                  {staffList.length === 0 && (
                    <p className='text-muted-foreground text-xs'>
                      {t('No staff members')}
                    </p>
                  )}
                </div>
              </div>
            )
          })}

          <Button onClick={saveAssignment} disabled={saving}>
            {saving ? t('Saving...') : t('Save')}
          </Button>
        </TabsContent>

        <TabsContent value='notification' className='space-y-4 pt-4'>
          <div className='flex items-center justify-between rounded-lg border p-4'>
            <div>
              <Label className='text-base'>
                {t('Email Notification')}
              </Label>
              <p className='text-muted-foreground text-sm'>
                {t('Send email when tickets are created or updated')}
              </p>
            </div>
            <Switch
              checked={notifyEnabled}
              onCheckedChange={setNotifyEnabled}
            />
          </div>
          <div className='space-y-1'>
            <Label>{t('Admin Email')}</Label>
            <Input
              value={adminEmail}
              onChange={(e) => setAdminEmail(e.target.value)}
              placeholder='admin@example.com'
            />
            <p className='text-muted-foreground text-xs'>
              {t('Separate multiple emails with semicolon')}
            </p>
          </div>
          <Button onClick={saveNotification} disabled={saving}>
            {saving ? t('Saving...') : t('Save')}
          </Button>
        </TabsContent>

        <TabsContent value='attachment' className='space-y-4 pt-4'>
          <div className='flex items-center justify-between rounded-lg border p-4'>
            <div>
              <Label className='text-base'>
                {t('Attachment Settings')}
              </Label>
              <p className='text-muted-foreground text-sm'>
                {t('Allow file attachments on tickets')}
              </p>
            </div>
            <Switch
              checked={attachEnabled}
              onCheckedChange={setAttachEnabled}
            />
          </div>

          <div className='grid gap-4 sm:grid-cols-2'>
            <div className='space-y-1'>
              <Label>{t('Max File Size (bytes)')}</Label>
              <Input
                type='number'
                value={maxSize}
                onChange={(e) => setMaxSize(e.target.value)}
              />
            </div>
            <div className='space-y-1'>
              <Label>{t('Max Attachments Per Ticket')}</Label>
              <Input
                type='number'
                value={maxCount}
                onChange={(e) => setMaxCount(e.target.value)}
              />
            </div>
            <div className='space-y-1'>
              <Label>{t('Allowed Extensions')}</Label>
              <Input
                value={allowedExts}
                onChange={(e) => setAllowedExts(e.target.value)}
                placeholder='.jpg,.png,.pdf'
              />
            </div>
            <div className='space-y-1'>
              <Label>{t('Allowed MIME Types')}</Label>
              <Input
                value={allowedMimes}
                onChange={(e) => setAllowedMimes(e.target.value)}
                placeholder='image/*,application/pdf'
              />
            </div>
          </div>

          <div className='space-y-2'>
            <Label>{t('Storage Backend')}</Label>
            <Select value={storage} onValueChange={setStorage}>
              <SelectTrigger className='w-40'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {STORAGE_BACKENDS.map((b) => (
                  <SelectItem key={b} value={b}>
                    {b.toUpperCase()}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {storage === 'local' && (
            <div className='space-y-1'>
              <Label>{t('Local Path')}</Label>
              <Input
                value={localPath}
                onChange={(e) => setLocalPath(e.target.value)}
                placeholder='/data/ticket-attachments'
              />
            </div>
          )}

          {storage !== 'local' && (
            <div className='grid gap-4 sm:grid-cols-2'>
              {cloudFields.map((field) => (
                <div key={field.key} className='space-y-1'>
                  <Label>{field.label}</Label>
                  <Input
                    type={field.secret ? 'password' : 'text'}
                    value={cloudConfig[field.key] ?? ''}
                    onChange={(e) =>
                      setCloudConfig((prev) => ({
                        ...prev,
                        [field.key]: e.target.value,
                      }))
                    }
                  />
                </div>
              ))}
              <div className='space-y-1'>
                <Label>{t('Signed URL TTL (seconds)')}</Label>
                <Input
                  type='number'
                  value={signedUrlTTL}
                  onChange={(e) => setSignedUrlTTL(e.target.value)}
                />
              </div>
            </div>
          )}

          <Button onClick={saveAttachment} disabled={saving}>
            {saving ? t('Saving...') : t('Save')}
          </Button>
        </TabsContent>
      </Tabs>
    </SettingsSection>
  )
}

function getCloudFields(
  storage: string
): Array<{ key: string; label: string; secret?: boolean }> {
  const prefix = 'TicketAttachment'
  switch (storage) {
    case 'oss':
      return [
        { key: `${prefix}OSSEndpoint`, label: 'Endpoint' },
        { key: `${prefix}OSSBucket`, label: 'Bucket' },
        { key: `${prefix}OSSRegion`, label: 'Region' },
        { key: `${prefix}OSSAccessKeyId`, label: 'Access Key ID', secret: true },
        { key: `${prefix}OSSAccessKeySecret`, label: 'Access Key Secret', secret: true },
        { key: `${prefix}OSSCustomDomain`, label: 'Custom Domain' },
      ]
    case 's3':
      return [
        { key: `${prefix}S3Endpoint`, label: 'Endpoint' },
        { key: `${prefix}S3Bucket`, label: 'Bucket' },
        { key: `${prefix}S3Region`, label: 'Region' },
        { key: `${prefix}S3AccessKeyId`, label: 'Access Key ID', secret: true },
        { key: `${prefix}S3AccessKeySecret`, label: 'Access Key Secret', secret: true },
        { key: `${prefix}S3CustomDomain`, label: 'Custom Domain' },
      ]
    case 'cos':
      return [
        { key: `${prefix}COSEndpoint`, label: 'Endpoint' },
        { key: `${prefix}COSBucket`, label: 'Bucket' },
        { key: `${prefix}COSRegion`, label: 'Region' },
        { key: `${prefix}COSSecretId`, label: 'Secret ID', secret: true },
        { key: `${prefix}COSSecretKey`, label: 'Secret Key', secret: true },
        { key: `${prefix}COSCustomDomain`, label: 'Custom Domain' },
      ]
    default:
      return []
  }
}
