import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { Languages } from 'lucide-react'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { ModelGroupSelector } from '@/components/model-group-selector'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import {
  getUserGroups,
  getUserModels,
} from '@/features/playground/api'
import { DEFAULT_GROUP } from '@/features/playground/constants'
import { INTERFACE_LANGUAGE_OPTIONS } from '@/i18n/languages'

import {
  buildTranslatePrompt,
  translateOnce,
} from './announcement-translate'

type TranslateResult = {
  contentI18n: Record<string, string>
  extraI18n: Record<string, string>
}

type AnnouncementTranslateDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  // 默认语言的原文（要翻译的来源）
  sourceContent: string
  sourceExtra: string
  // 现有译文，用于"仅填空白"模式判断
  currentContentI18n: Record<string, string>
  currentExtraI18n: Record<string, string>
  onApply: (result: TranslateResult) => void
}

export function AnnouncementTranslateDialog({
  open,
  onOpenChange,
  sourceContent,
  sourceExtra,
  currentContentI18n,
  currentExtraI18n,
  onApply,
}: AnnouncementTranslateDialogProps) {
  const { t } = useTranslation()
  const [group, setGroup] = useState<string>(DEFAULT_GROUP)
  const [model, setModel] = useState('')
  const [overwrite, setOverwrite] = useState(false)
  const [running, setRunning] = useState(false)
  const [progress, setProgress] = useState({ done: 0, total: 0 })

  const { data: groups = [] } = useQuery({
    queryKey: ['pg-groups'],
    queryFn: getUserGroups,
    enabled: open,
  })
  const { data: models = [] } = useQuery({
    queryKey: ['pg-models', group],
    queryFn: () => getUserModels(group),
    enabled: open && group !== '',
  })

  useEffect(() => {
    if (!open) {
      setRunning(false)
      setProgress({ done: 0, total: 0 })
    }
  }, [open])

  // group 变化后若当前模型不在新列表里则重置
  useEffect(() => {
    if (models.length > 0 && !models.some((m) => m.value === model)) {
      setModel(models[0].value)
    }
  }, [models, model])

  const canRun = useMemo(
    () => !!model && !!sourceContent.trim() && !running,
    [model, sourceContent, running]
  )

  const handleTranslate = async () => {
    if (!model) {
      toast.error(t('Please select a model'))
      return
    }
    // 目标语言：跳过留空原文对应无意义的情况；逐语言 content(+extra) 顺序调用
    const targets = INTERFACE_LANGUAGE_OPTIONS
    const nextContent: Record<string, string> = { ...currentContentI18n }
    const nextExtra: Record<string, string> = { ...currentExtraI18n }
    const hasExtra = sourceExtra.trim() !== ''

    // 计算需要翻译的任务数（content 每语言 1 次；extra 有原文再 1 次）
    const tasks: Array<{ code: string; label: string; field: 'content' | 'extra' }> =
      []
    for (const lang of targets) {
      const contentFilled = (currentContentI18n[lang.code] ?? '').trim() !== ''
      if (overwrite || !contentFilled) {
        tasks.push({ code: lang.code, label: lang.label, field: 'content' })
      }
      if (hasExtra) {
        const extraFilled = (currentExtraI18n[lang.code] ?? '').trim() !== ''
        if (overwrite || !extraFilled) {
          tasks.push({ code: lang.code, label: lang.label, field: 'extra' })
        }
      }
    }

    if (tasks.length === 0) {
      toast.info(t('All languages already have translations'))
      return
    }

    setRunning(true)
    setProgress({ done: 0, total: tasks.length })
    let failed = 0
    for (let i = 0; i < tasks.length; i++) {
      const task = tasks[i]
      const source = task.field === 'content' ? sourceContent : sourceExtra
      try {
        const text = await translateOnce({
          model,
          group,
          prompt: buildTranslatePrompt(source, task.label),
        })
        if (text) {
          if (task.field === 'content') nextContent[task.code] = text
          else nextExtra[task.code] = text
        } else {
          failed++
        }
      } catch {
        failed++
        toast.error(
          t('Failed to translate into {{lang}}', { lang: task.label })
        )
      }
      setProgress({ done: i + 1, total: tasks.length })
    }

    setRunning(false)
    onApply({ contentI18n: nextContent, extraI18n: nextExtra })
    if (failed === 0) {
      toast.success(t('Translation completed'))
      onOpenChange(false)
    } else {
      toast.warning(
        t('Translation finished with {{n}} failure(s)', { n: failed })
      )
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={
        <span className='flex items-center gap-2'>
          <Languages className='size-4' />
          {t('AI Translate')}
        </span>
      }
      description={t(
        'Translate the default content into all interface languages using a model'
      )}
      contentClassName='sm:max-w-lg'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={running}
          >
            {t('Cancel')}
          </Button>
          <Button type='button' onClick={handleTranslate} disabled={!canRun}>
            {running
              ? t('Translating {{done}}/{{total}}', progress)
              : t('Start Translation')}
          </Button>
        </>
      }
    >
      <div className='space-y-2'>
        <Label>{t('Group and Model')}</Label>
        <ModelGroupSelector
          selectedModel={model}
          models={models}
          onModelChange={setModel}
          selectedGroup={group}
          groups={groups}
          onGroupChange={setGroup}
          disabled={running}
        />
      </div>
      <label className='flex items-center gap-2 text-sm'>
        <Checkbox
          checked={overwrite}
          onCheckedChange={(v) => setOverwrite(v === true)}
          disabled={running}
        />
        {t('Overwrite existing translations')}
      </label>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Uses your account quota, same as the playground. Languages are translated one by one.'
        )}
      </p>
    </Dialog>
  )
}
