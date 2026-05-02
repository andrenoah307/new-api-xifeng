import { useState, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { RotateCcw, Eye } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import { api } from '@/lib/api'

interface TemplateVariable {
  name: string
  description: string
}

interface EmailTemplate {
  key: string
  name: string
  description: string
  customized: boolean
  default_subject: string
  default_body: string
  current_subject: string
  current_body: string
  variables: TemplateVariable[]
}

async function getEmailTemplates(): Promise<EmailTemplate[]> {
  const res = await api.get('/api/option/email_templates')
  return res.data?.data ?? []
}

async function previewTemplate(
  key: string,
  subject: string,
  body: string
): Promise<{ subject: string; body: string }> {
  const res = await api.post('/api/option/email_templates/preview', {
    key,
    subject,
    body,
  })
  return res.data?.data ?? { subject: '', body: '' }
}

async function resetTemplate(key: string): Promise<void> {
  await api.post('/api/option/email_templates/reset', { key })
}

export function EmailTemplateSettingsSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const updateOption = useUpdateOption()
  const bodyRef = useRef<HTMLTextAreaElement>(null)

  const { data: templates = [], isLoading } = useQuery({
    queryKey: ['email-templates'],
    queryFn: getEmailTemplates,
  })

  const [selectedKey, setSelectedKey] = useState<string>('')
  const [draftSubject, setDraftSubject] = useState('')
  const [draftBody, setDraftBody] = useState('')
  const [previewHtml, setPreviewHtml] = useState<string | null>(null)
  const [showPreview, setShowPreview] = useState(false)
  const [showReset, setShowReset] = useState(false)
  const [saving, setSaving] = useState(false)

  const current = templates.find((t) => t.key === selectedKey)

  const selectTemplate = (key: string) => {
    const tpl = templates.find((t) => t.key === key)
    if (tpl) {
      setSelectedKey(key)
      setDraftSubject(tpl.current_subject)
      setDraftBody(tpl.current_body)
    }
  }

  if (!selectedKey && templates.length > 0) {
    selectTemplate(templates[0].key)
  }

  const insertVariable = (name: string) => {
    const tag = `{{${name}}}`
    const el = bodyRef.current
    if (el) {
      const start = el.selectionStart
      const end = el.selectionEnd
      const before = draftBody.slice(0, start)
      const after = draftBody.slice(end)
      setDraftBody(before + tag + after)
      requestAnimationFrame(() => {
        el.selectionStart = el.selectionEnd = start + tag.length
        el.focus()
      })
    } else {
      setDraftBody(draftBody + tag)
    }
  }

  const handleSave = async () => {
    if (!current) return
    setSaving(true)
    try {
      const subjectValue =
        draftSubject === current.default_subject ? '' : draftSubject
      const bodyValue = draftBody === current.default_body ? '' : draftBody

      await updateOption.mutateAsync({
        key: `EmailTemplate.${current.key}.subject`,
        value: subjectValue,
      })
      await updateOption.mutateAsync({
        key: `EmailTemplate.${current.key}.body`,
        value: bodyValue,
      })
      queryClient.invalidateQueries({ queryKey: ['email-templates'] })
      toast.success(t('Config saved'))
    } catch {
      toast.error(t('Operation failed'))
    } finally {
      setSaving(false)
    }
  }

  const handlePreview = async () => {
    if (!current) return
    try {
      const result = await previewTemplate(
        current.key,
        draftSubject,
        draftBody
      )
      setPreviewHtml(result.body)
      setShowPreview(true)
    } catch {
      toast.error(t('Operation failed'))
    }
  }

  const handleReset = async () => {
    if (!current) return
    try {
      await resetTemplate(current.key)
      queryClient.invalidateQueries({ queryKey: ['email-templates'] })
      setDraftSubject(current.default_subject)
      setDraftBody(current.default_body)
      setShowReset(false)
      toast.success(t('Config saved'))
    } catch {
      toast.error(t('Operation failed'))
    }
  }

  if (isLoading) {
    return (
      <SettingsSection
        title={t('Email Templates')}
        description={t('Configure email notification templates')}
      >
        <div className='text-muted-foreground py-8 text-center text-sm'>
          {t('Loading settings...')}
        </div>
      </SettingsSection>
    )
  }

  return (
    <SettingsSection
      title={t('Email Templates')}
      description={t('Configure email notification templates')}
    >
      <div className='space-y-4'>
        <div className='flex items-center gap-3'>
          <Select value={selectedKey} onValueChange={selectTemplate}>
            <SelectTrigger className='w-64'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {templates.map((tpl) => (
                <SelectItem key={tpl.key} value={tpl.key}>
                  {tpl.name}
                  {tpl.customized && (
                    <Badge variant='outline' className='ml-2 text-[10px]'>
                      {t('Customized')}
                    </Badge>
                  )}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {current?.customized && (
            <Button
              variant='outline'
              size='sm'
              onClick={() => setShowReset(true)}
            >
              <RotateCcw className='mr-1 h-3.5 w-3.5' />
              {t('Reset to Default')}
            </Button>
          )}
        </div>

        {current && (
          <p className='text-muted-foreground text-sm'>{current.description}</p>
        )}

        {current?.variables && current.variables.length > 0 && (
          <div className='space-y-1'>
            <Label className='text-xs'>{t('Insert Variable')}</Label>
            <div className='flex flex-wrap gap-1.5'>
              {current.variables.map((v) => (
                <Badge
                  key={v.name}
                  variant='secondary'
                  className='cursor-pointer text-xs'
                  onClick={() => insertVariable(v.name)}
                  title={v.description}
                >
                  {`{{${v.name}}}`}
                </Badge>
              ))}
            </div>
          </div>
        )}

        <div className='space-y-1'>
          <Label>{t('Subject')}</Label>
          <Input
            value={draftSubject}
            onChange={(e) => setDraftSubject(e.target.value)}
          />
        </div>

        <div className='space-y-1'>
          <Label>{t('Body')}</Label>
          <Textarea
            ref={bodyRef}
            value={draftBody}
            onChange={(e) => setDraftBody(e.target.value)}
            rows={16}
            className='font-mono text-xs'
          />
        </div>

        <div className='flex gap-2'>
          <Button onClick={handleSave} disabled={saving}>
            {saving ? t('Saving...') : t('Save')}
          </Button>
          <Button variant='outline' onClick={handlePreview}>
            <Eye className='mr-1 h-3.5 w-3.5' />
            {t('Template Preview')}
          </Button>
        </div>
      </div>

      <Dialog open={showPreview} onOpenChange={setShowPreview}>
        <DialogContent className='max-w-2xl'>
          <DialogHeader>
            <DialogTitle>{t('Template Preview')}</DialogTitle>
          </DialogHeader>
          {previewHtml && (
            <iframe
              srcDoc={previewHtml}
              className='h-[60vh] w-full rounded border'
              sandbox='allow-same-origin'
              title='preview'
            />
          )}
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={showReset}
        onOpenChange={setShowReset}
        title={t('Reset to Default')}
        desc={t(
          'This will reset the template to the system default. Continue?'
        )}
        handleConfirm={handleReset}
      />
    </SettingsSection>
  )
}
