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
import { zodResolver } from '@hookform/resolvers/zod'
import { Plus, Trash2, Save } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { StaticRowActions } from '@/components/data-table/static/static-row-actions'
import { DateTimePicker } from '@/components/datetime-picker'
import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { INTERFACE_LANGUAGE_OPTIONS } from '@/i18n/languages'
import dayjs from '@/lib/dayjs'

import { SettingsSwitchField } from '../components/settings-form-layout'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

type Announcement = {
  id: number
  content: string
  publishDate: string
  type: 'default' | 'ongoing' | 'success' | 'warning' | 'error'
  extra?: string
  // 按界面语言码存的翻译，留空的语言在用户侧回退 content/extra
  contentI18n?: Record<string, string>
  extraI18n?: Record<string, string>
}

type AnnouncementsSectionProps = {
  enabled: boolean
  data: string
}

const announcementSchema = z.object({
  content: z.string().min(1, 'Content is required'),
  publishDate: z.string().min(1, 'Publish date is required'),
  type: z.enum(['default', 'ongoing', 'success', 'warning', 'error']),
  extra: z
    .string()
    .max(100, 'Extra must be less than 100 characters')
    .optional(),
  contentI18n: z.record(z.string(), z.string()).optional(),
  extraI18n: z.record(z.string(), z.string()).optional(),
})

type AnnouncementFormValues = z.infer<typeof announcementSchema>

const ANNOUNCEMENT_FORM_ID = 'announcement-form'

// 所有语言键都预置空串，保证语言 Tab 里的输入始终受控
const emptyI18nMap = (): Record<string, string> =>
  Object.fromEntries(INTERFACE_LANGUAGE_OPTIONS.map((o) => [o.code, '']))

// 保存前清洗：去掉留空的语言；全空则整个字段置 undefined（JSON 序列化时丢弃）
const cleanI18nMap = (
  map: Record<string, string> | undefined
): Record<string, string> | undefined => {
  if (!map) return undefined
  const entries = Object.entries(map).filter(([, v]) => v.trim() !== '')
  return entries.length > 0 ? Object.fromEntries(entries) : undefined
}

const typeOptions = [
  {
    value: 'default',
    label: 'Default',
    color: 'bg-gray-500',
    badgeVariant: 'neutral' as const,
  },
  {
    value: 'ongoing',
    label: 'Ongoing',
    color: 'bg-blue-500',
    badgeVariant: 'info' as const,
  },
  {
    value: 'success',
    label: 'Success',
    color: 'bg-green-500',
    badgeVariant: 'success' as const,
  },
  {
    value: 'warning',
    label: 'Warning',
    color: 'bg-orange-500',
    badgeVariant: 'warning' as const,
  },
  {
    value: 'error',
    label: 'Error',
    color: 'bg-red-500',
    badgeVariant: 'danger' as const,
  },
]

export function AnnouncementsSection({
  enabled,
  data,
}: AnnouncementsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [announcements, setAnnouncements] = useState<Announcement[]>([])
  const [isEnabled, setIsEnabled] = useState(enabled)
  const [hasChanges, setHasChanges] = useState(false)
  const [selectedIds, setSelectedIds] = useState<number[]>([])
  const [showDialog, setShowDialog] = useState(false)
  const [showDeleteDialog, setShowDeleteDialog] = useState(false)
  const [editingAnnouncement, setEditingAnnouncement] =
    useState<Announcement | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<'single' | 'batch'>('single')
  const [langTab, setLangTab] = useState('default')

  const form = useForm<AnnouncementFormValues>({
    resolver: zodResolver(announcementSchema),
    defaultValues: {
      content: '',
      publishDate: new Date().toISOString(),
      type: 'default',
      extra: '',
      contentI18n: emptyI18nMap(),
      extraI18n: emptyI18nMap(),
    },
  })

  const watchContentI18n = form.watch('contentI18n')
  const watchExtraI18n = form.watch('extraI18n')

  useEffect(() => {
    try {
      const parsed = JSON.parse(data || '[]')
      if (Array.isArray(parsed)) {
        setAnnouncements(
          parsed.map((item, idx) => ({
            ...item,
            id: item.id || idx + 1,
          }))
        )
      }
    } catch {
      setAnnouncements([])
    }
  }, [data])

  useEffect(() => {
    setIsEnabled(enabled)
  }, [enabled])

  const handleToggleEnabled = async (checked: boolean) => {
    try {
      await updateOption.mutateAsync({
        key: 'console_setting.announcements_enabled',
        value: checked,
      })
      setIsEnabled(checked)
      toast.success(t('Setting saved'))
    } catch {
      toast.error(t('Failed to update setting'))
    }
  }

  const handleAdd = () => {
    setEditingAnnouncement(null)
    form.reset({
      content: '',
      publishDate: new Date().toISOString(),
      type: 'default',
      extra: '',
      contentI18n: emptyI18nMap(),
      extraI18n: emptyI18nMap(),
    })
    setLangTab('default')
    setShowDialog(true)
  }

  const handleEdit = (announcement: Announcement) => {
    setEditingAnnouncement(announcement)
    form.reset({
      content: announcement.content,
      publishDate: announcement.publishDate,
      type: announcement.type,
      extra: announcement.extra || '',
      contentI18n: { ...emptyI18nMap(), ...(announcement.contentI18n ?? {}) },
      extraI18n: { ...emptyI18nMap(), ...(announcement.extraI18n ?? {}) },
    })
    setLangTab('default')
    setShowDialog(true)
  }

  const handleDelete = (announcement: Announcement) => {
    setEditingAnnouncement(announcement)
    setDeleteTarget('single')
    setShowDeleteDialog(true)
  }

  const handleBatchDelete = () => {
    if (selectedIds.length === 0) {
      toast.error(t('Please select items to delete'))
      return
    }
    setDeleteTarget('batch')
    setShowDeleteDialog(true)
  }

  const confirmDelete = () => {
    if (deleteTarget === 'single' && editingAnnouncement) {
      setAnnouncements((prev) =>
        prev.filter((item) => item.id !== editingAnnouncement.id)
      )
      setHasChanges(true)
      toast.success(t('Announcement deleted. Click "Save Settings" to apply.'))
    } else if (deleteTarget === 'batch') {
      setAnnouncements((prev) =>
        prev.filter((item) => !selectedIds.includes(item.id))
      )
      setSelectedIds([])
      setHasChanges(true)
      toast.success(
        t('{{count}} announcements deleted. Click "Save Settings" to apply.', {
          count: selectedIds.length,
        })
      )
    }
    setShowDeleteDialog(false)
    setEditingAnnouncement(null)
  }

  const handleSubmitForm = (values: AnnouncementFormValues) => {
    // 显式携带 undefined 覆盖旧值，JSON.stringify 会丢弃该键，旧数据形态不被污染
    const cleaned = {
      ...values,
      contentI18n: cleanI18nMap(values.contentI18n),
      extraI18n: cleanI18nMap(values.extraI18n),
    }
    if (editingAnnouncement) {
      setAnnouncements((prev) =>
        prev.map((item) =>
          item.id === editingAnnouncement.id ? { ...item, ...cleaned } : item
        )
      )
      toast.success(t('Announcement updated. Click "Save Settings" to apply.'))
    } else {
      const newId = Math.max(...announcements.map((item) => item.id), 0) + 1
      setAnnouncements((prev) => [...prev, { id: newId, ...cleaned }])
      toast.success(t('Announcement added. Click "Save Settings" to apply.'))
    }
    setHasChanges(true)
    setShowDialog(false)
  }

  const handleSaveAll = async () => {
    try {
      await updateOption.mutateAsync({
        key: 'console_setting.announcements',
        value: JSON.stringify(announcements),
      })
      setHasChanges(false)
      toast.success(t('Announcements saved successfully'))
    } catch {
      toast.error(t('Failed to save announcements'))
    }
  }

  const toggleSelectAll = (checked: boolean) => {
    setSelectedIds(checked ? announcements.map((item) => item.id) : [])
  }

  const toggleSelectOne = (id: number, checked: boolean) => {
    setSelectedIds((prev) =>
      checked ? [...prev, id] : prev.filter((item) => item !== id)
    )
  }

  const sortedAnnouncements = useMemo(() => {
    return [...announcements].sort((a, b) => {
      return (
        new Date(b.publishDate).getTime() - new Date(a.publishDate).getTime()
      )
    })
  }, [announcements])

  const getRelativeTime = (date: string) => {
    const now = new Date()
    const past = new Date(date)
    const diffMs = now.getTime() - past.getTime()
    const diffMins = Math.floor(diffMs / 60000)
    const diffHours = Math.floor(diffMins / 60)
    const diffDays = Math.floor(diffHours / 24)

    if (diffMins < 60) return `${diffMins}m ago`
    if (diffHours < 24) return `${diffHours}h ago`
    return `${diffDays}d ago`
  }

  return (
    <SettingsSection title={t('Announcements')}>
      <div className='space-y-4'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <div className='flex flex-wrap items-center gap-2'>
            <Button onClick={handleAdd} size='sm'>
              <Plus className='mr-2 h-4 w-4' />
              {t('Add Announcement')}
            </Button>
            <Button
              onClick={handleBatchDelete}
              size='sm'
              variant='destructive'
              disabled={selectedIds.length === 0}
            >
              <Trash2 className='mr-2 h-4 w-4' />
              {t('Delete (')}
              {selectedIds.length})
            </Button>
            <Button
              onClick={handleSaveAll}
              size='sm'
              variant='secondary'
              disabled={!hasChanges || updateOption.isPending}
            >
              <Save className='mr-2 h-4 w-4' />
              {updateOption.isPending ? t('Saving...') : t('Save Settings')}
            </Button>
          </div>
          <SettingsSwitchField
            checked={isEnabled}
            onCheckedChange={handleToggleEnabled}
            label={t('Enabled')}
            className='py-0'
          />
        </div>

        <StaticDataTable
          data={sortedAnnouncements}
          getRowKey={(announcement) => announcement.id}
          emptyContent={t(
            'No announcements yet. Click "Add Announcement" to create one.'
          )}
          columns={[
            {
              id: 'select',
              header: (
                <Checkbox
                  checked={
                    selectedIds.length === announcements.length &&
                    announcements.length > 0
                  }
                  onCheckedChange={toggleSelectAll}
                />
              ),
              className: 'w-12',
              cell: (announcement) => (
                <Checkbox
                  checked={selectedIds.includes(announcement.id)}
                  onCheckedChange={(checked) =>
                    toggleSelectOne(announcement.id, checked as boolean)
                  }
                />
              ),
            },
            {
              id: 'content',
              header: t('Content'),
              cellClassName: 'max-w-xs truncate',
              cell: (announcement) => announcement.content,
            },
            {
              id: 'publish-date',
              header: t('Publish Date'),
              cell: (announcement) => (
                <div className='flex flex-col gap-1'>
                  <span className='text-sm font-medium'>
                    {getRelativeTime(announcement.publishDate)}
                  </span>
                  <span className='text-muted-foreground text-xs'>
                    {dayjs(announcement.publishDate).format(
                      'YYYY-MM-DD HH:mm:ss'
                    )}
                  </span>
                </div>
              ),
            },
            {
              id: 'type',
              header: t('Type'),
              cell: (announcement) => (
                <StatusBadge
                  label={
                    typeOptions.find((opt) => opt.value === announcement.type)
                      ?.label
                  }
                  variant={
                    typeOptions.find((opt) => opt.value === announcement.type)
                      ?.badgeVariant ?? 'neutral'
                  }
                  copyable={false}
                />
              ),
            },
            {
              id: 'extra',
              header: t('Extra'),
              cellClassName: 'text-muted-foreground max-w-xs truncate',
              cell: (announcement) => announcement.extra || '-',
            },
            {
              id: 'actions',
              header: t('Actions'),
              cell: (announcement) => (
                <StaticRowActions
                  editLabel={t('Edit')}
                  deleteLabel={t('Delete')}
                  menuLabel={t('Open menu')}
                  onEdit={() => handleEdit(announcement)}
                  onDelete={() => handleDelete(announcement)}
                />
              ),
            },
          ]}
        />
      </div>

      <Dialog
        open={showDialog}
        onOpenChange={setShowDialog}
        title={
          editingAnnouncement ? t('Edit Announcement') : t('Add Announcement')
        }
        description={t(
          'Create or update system announcements for the dashboard'
        )}
        contentClassName='max-w-2xl'
        contentHeight='auto'
        bodyClassName='space-y-4'
        footer={
          <>
            <Button
              type='button'
              variant='outline'
              onClick={() => setShowDialog(false)}
            >
              {t('Cancel')}
            </Button>
            <Button type='submit' form={ANNOUNCEMENT_FORM_ID}>
              {editingAnnouncement ? t('Update') : t('Add')}
            </Button>
          </>
        }
      >
        <Form {...form}>
          <form
            id={ANNOUNCEMENT_FORM_ID}
            onSubmit={form.handleSubmit(handleSubmitForm)}
            className='space-y-4'
          >
            <Tabs value={langTab} onValueChange={(v) => setLangTab(String(v))}>
              <TabsList className='h-auto max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto'>
                <TabsTrigger value='default'>{t('Default')}</TabsTrigger>
                {INTERFACE_LANGUAGE_OPTIONS.map((option) => (
                  <TabsTrigger key={option.code} value={option.code}>
                    {option.label}
                    {(watchContentI18n?.[option.code]?.trim() ||
                      watchExtraI18n?.[option.code]?.trim()) && (
                      <span className='bg-primary ml-1 inline-block size-1.5 rounded-full' />
                    )}
                  </TabsTrigger>
                ))}
              </TabsList>
              <TabsContent value='default' className='space-y-4 pt-2'>
                <FormField
                  control={form.control}
                  name='content'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Content')}</FormLabel>
                      <FormControl>
                        <Textarea
                          placeholder={t(
                            'Enter announcement content (supports Markdown/HTML)'
                          )}
                          rows={4}
                          {...field}
                        />
                      </FormControl>
                      <FormDescription>
                        {t('Supports Markdown and HTML.')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='extra'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Extra Notes (Optional)')}</FormLabel>
                      <FormControl>
                        <Input
                          placeholder={t('Additional information')}
                          {...field}
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Optional supplementary information (max 100 characters)'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </TabsContent>
              {INTERFACE_LANGUAGE_OPTIONS.map((option) => (
                <TabsContent
                  key={option.code}
                  value={option.code}
                  className='space-y-4 pt-2'
                >
                  <FormField
                    control={form.control}
                    name={`contentI18n.${option.code}`}
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          {t('Content')} · {option.label}
                        </FormLabel>
                        <FormControl>
                          <Textarea
                            placeholder={t(
                              'Leave empty to fall back to the default content'
                            )}
                            rows={4}
                            {...field}
                            value={field.value ?? ''}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name={`extraI18n.${option.code}`}
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          {t('Extra Notes (Optional)')} · {option.label}
                        </FormLabel>
                        <FormControl>
                          <Input
                            placeholder={t(
                              'Leave empty to fall back to the default content'
                            )}
                            {...field}
                            value={field.value ?? ''}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </TabsContent>
              ))}
            </Tabs>
            <FormField
              control={form.control}
              name='publishDate'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Publish Date')}</FormLabel>
                  <FormControl>
                    <DateTimePicker
                      value={field.value ? new Date(field.value) : undefined}
                      onChange={(date) =>
                        field.onChange(date ? date.toISOString() : '')
                      }
                      placeholder={t('Select publish date')}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Date and time when this announcement should be displayed'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='type'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Type')}</FormLabel>
                  <Select
                    items={typeOptions.map((option) => ({
                      value: option.value,
                      label: (
                        <div className='flex items-center gap-2'>
                          <div
                            className={`h-3 w-3 rounded-full ${option.color}`}
                          />
                          {option.label}
                        </div>
                      ),
                    }))}
                    onValueChange={field.onChange}
                    value={field.value}
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue
                          placeholder={t('Select announcement type')}
                        />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        {typeOptions.map((option) => (
                          <SelectItem key={option.value} value={option.value}>
                            <div className='flex items-center gap-2'>
                              <div
                                className={`h-3 w-3 rounded-full ${option.color}`}
                              />
                              {option.label}
                            </div>
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />
          </form>
        </Form>
      </Dialog>

      <AlertDialog open={showDeleteDialog} onOpenChange={setShowDeleteDialog}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Are you sure?')}</AlertDialogTitle>
            <AlertDialogDescription>
              {deleteTarget === 'single'
                ? t('This announcement will be removed from the list.')
                : t('{{count}} announcements will be removed from the list.', {
                    count: selectedIds.length,
                  })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
            <AlertDialogAction variant='destructive' onClick={confirmDelete}>
              {t('Delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </SettingsSection>
  )
}
