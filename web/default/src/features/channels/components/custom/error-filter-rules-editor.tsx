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
*/
import {
  AlertTriangle,
  ChevronDown,
  ChevronUp,
  Filter,
  History,
  Plus,
  RefreshCw,
  Trash2,
} from 'lucide-react'
import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { TagInput } from '@/components/tag-input'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { getAllLogs } from '@/features/usage-logs/api'
import { formatTimestampToDate } from '@/lib/format'

import type { ChannelFormValues } from '../../lib/channel-form'
import {
  deduplicateErrors,
  hasCondition,
  normalizeRule,
  normalizeStatusCodes,
  normalizeStringList,
  parseErrorLog,
  parseRules,
  stripErrorContentPrefix,
  type ErrorLogInput,
  type FilterRule,
  type ParsedErrorLog,
} from '../../lib/error-filter'

export type { FilterRule, ParsedErrorLog } from '../../lib/error-filter'

const STATUS_CODE_SEPARATORS = [',', '，', ' ']
const STRING_SEPARATORS = [',', '，']

function normalizeStatusTag(raw: string): string | null {
  const [statusCode] = normalizeStatusCodes([raw])
  return statusCode === undefined ? null : String(statusCode)
}

function normalizeStringTag(raw: string): string | null {
  return normalizeStringList([raw])[0] ?? null
}

interface RuleSummaryData {
  action: FilterRule['action']
  conditions: string[]
  hasCondition: boolean
}

function formatRuleSummary(rule: FilterRule): RuleSummaryData {
  const conditions: string[] = []
  if (rule.status_codes.length > 0) {
    conditions.push(rule.status_codes.map(String).join(' / '))
  }
  if (rule.message_contains.length > 0) {
    const preview = rule.message_contains.slice(0, 2).join('", "')
    conditions.push(
      `"${preview}${rule.message_contains.length > 2 ? '"…' : '"'}`
    )
  }
  if (rule.error_codes.length > 0) {
    conditions.push(
      rule.error_codes.slice(0, 2).join(', ') +
        (rule.error_codes.length > 2 ? '…' : '')
    )
  }

  return {
    action: rule.action,
    conditions,
    hasCondition: hasCondition(rule),
  }
}

function applyRecentErrorsToRule(
  rule: FilterRule,
  selected: readonly ParsedErrorLog[]
): FilterRule {
  const statusCodes = normalizeStatusCodes([
    ...rule.status_codes,
    ...selected.map((error) => error.statusCode),
  ])
  const errorCodes = normalizeStringList([
    ...rule.error_codes,
    ...selected.map((error) => error.errorCode),
  ])
  const messages = normalizeStringList([
    ...rule.message_contains,
    ...selected.map((error) => stripErrorContentPrefix(error.content)),
  ])

  return {
    ...rule,
    status_codes: statusCodes,
    error_codes: errorCodes,
    message_contains: messages,
  }
}

function formatErrorTime(value: ParsedErrorLog['createdAt']): string {
  const timestamp = typeof value === 'number' ? value : Number(value)
  return Number.isFinite(timestamp) && timestamp > 0
    ? formatTimestampToDate(timestamp)
    : '-'
}

function errorKey(error: ParsedErrorLog, index: number): string {
  if (error.id !== undefined && error.id !== null) return String(error.id)
  return `${error.createdAt ?? ''}|${error.statusCode ?? ''}|${error.errorCode}|${error.content}|${index}`
}

interface RecentErrorsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelId: number
  onApply: (errors: ParsedErrorLog[]) => void
}

function RecentErrorsDialog({
  open,
  onOpenChange,
  channelId,
  onApply,
}: RecentErrorsDialogProps) {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const [loadError, setLoadError] = useState(false)
  const [errors, setErrors] = useState<ParsedErrorLog[]>([])
  const [selectedKeys, setSelectedKeys] = useState<Set<string>>(new Set())

  const fetchErrors = useCallback(async () => {
    setLoading(true)
    setLoadError(false)
    try {
      const response = await getAllLogs({
        type: 5,
        channel: channelId,
        p: 1,
        page_size: 50,
        total_count: 50,
      })
      if (!response.success) throw new Error('request failed')

      const items = Array.isArray(response.data?.items)
        ? response.data.items
        : []
      const parsed = items.map((item) => parseErrorLog(item as ErrorLogInput))
      const displayReady = parsed.map((error) => ({
        ...error,
        content: stripErrorContentPrefix(error.content),
      }))
      setErrors(deduplicateErrors(displayReady))
    } catch {
      setErrors([])
      setLoadError(true)
      toast.error(t('Failed to load recent error records'))
    } finally {
      setLoading(false)
    }
  }, [channelId, t])

  useEffect(() => {
    if (!open) return
    setSelectedKeys(new Set())
    void fetchErrors()
  }, [fetchErrors, open])

  const toggleSelected = (key: string) => {
    setSelectedKeys((previous) => {
      const next = new Set(previous)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  const handleApply = () => {
    const selected = errors.filter((error, index) =>
      selectedKeys.has(errorKey(error, index))
    )
    if (selected.length > 0) onApply(selected)
    onOpenChange(false)
  }

  let errorContent: ReactNode
  if (loading) {
    errorContent = (
      <p className='text-muted-foreground py-8 text-center text-sm'>
        {t('Loading...')}
      </p>
    )
  } else if (loadError) {
    errorContent = (
      <p className='text-destructive py-8 text-center text-sm'>
        {t('Failed to load recent error records')}
      </p>
    )
  } else if (errors.length === 0) {
    errorContent = (
      <p className='text-muted-foreground py-8 text-center text-sm'>
        {t('No recent error records')}
      </p>
    )
  } else {
    errorContent = (
      <div className='space-y-2'>
        {errors.map((error, index) => {
          const key = errorKey(error, index)
          const checked = selectedKeys.has(key)
          const message = stripErrorContentPrefix(error.content)
          return (
            <div
              key={key}
              role='button'
              tabIndex={0}
              data-error-record='true'
              aria-pressed={checked}
              onClick={() => toggleSelected(key)}
              onKeyDown={(event) => {
                if (event.key === 'Enter' || event.key === ' ') {
                  event.preventDefault()
                  toggleSelected(key)
                }
              }}
              className={`cursor-pointer rounded-lg border px-3 py-2 transition-colors ${
                checked
                  ? 'border-primary bg-primary/5'
                  : 'border-border hover:bg-muted/50'
              }`}
            >
              <div className='flex min-w-0 items-start gap-3'>
                <Checkbox
                  checked={checked}
                  onCheckedChange={() => toggleSelected(key)}
                  onClick={(event) => event.stopPropagation()}
                  aria-label={t('Select')}
                  className='mt-0.5'
                />
                <div className='min-w-0 flex-1 space-y-1'>
                  <div className='text-muted-foreground flex flex-wrap items-center gap-2 text-xs'>
                    {error.statusCode !== null && (
                      <span className='text-foreground font-medium'>
                        {error.statusCode}
                      </span>
                    )}
                    {error.errorCode && (
                      <span className='text-foreground font-medium'>
                        {error.errorCode}
                      </span>
                    )}
                    <time dateTime={String(error.createdAt ?? '')}>
                      {formatErrorTime(error.createdAt)}
                    </time>
                  </div>
                  <p className='line-clamp-2 text-sm break-words whitespace-pre-wrap'>
                    {message || (
                      <span className='text-muted-foreground'>
                        {t('No message content')}
                      </span>
                    )}
                  </p>
                </div>
              </div>
            </div>
          )
        })}
      </div>
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-h-[88vh] gap-3 sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle className='flex items-center gap-2'>
            <History className='size-4' aria-hidden='true' />
            {t('Recent upstream errors')}
          </DialogTitle>
          <DialogDescription>
            {t(
              'Recent error records are limited to 50 entries and deduplicated.'
            )}
          </DialogDescription>
        </DialogHeader>

        <div className='flex items-center justify-end'>
          <Button
            type='button'
            variant='ghost'
            size='sm'
            onClick={() => void fetchErrors()}
            disabled={loading}
            title={t('Refresh')}
          >
            <RefreshCw
              className={loading ? 'animate-spin' : ''}
              aria-hidden='true'
            />
            {t('Refresh')}
          </Button>
        </div>

        <ScrollArea className='max-h-[min(60vh,36rem)] pr-3'>
          {errorContent}
        </ScrollArea>

        <DialogFooter className='flex-row items-center justify-between sm:justify-between'>
          <span className='text-muted-foreground text-xs'>
            {t('Selected {{count}}', { count: selectedKeys.size })}
          </span>
          <div className='flex gap-2'>
            <Button
              type='button'
              variant='outline'
              onClick={() => onOpenChange(false)}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='button'
              onClick={handleApply}
              disabled={selectedKeys.size === 0}
            >
              {t('Apply selected')}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

interface RuleItemProps {
  rule: FilterRule
  index: number
  channelId?: number
  onUpdate: (index: number, patch: Partial<FilterRule>) => void
  onRemove: (index: number) => void
}

function RuleSummary({ rule }: { rule: FilterRule }) {
  const { t } = useTranslation()
  const summary = formatRuleSummary(rule)
  const actionLabels: Record<FilterRule['action'], string> = {
    retry: t('Retry'),
    rewrite: t('Rewrite'),
    replace: t('Replace'),
  }
  const actionClasses: Record<FilterRule['action'], string> = {
    retry: 'border-blue-500/30 bg-blue-500/10 text-blue-700 dark:text-blue-300',
    rewrite:
      'border-orange-500/30 bg-orange-500/10 text-orange-700 dark:text-orange-300',
    replace: 'border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300',
  }

  return (
    <div className='flex min-w-0 items-center gap-2 overflow-hidden whitespace-nowrap'>
      <Badge variant='outline' className={actionClasses[summary.action]}>
        {actionLabels[summary.action]}
      </Badge>
      {summary.hasCondition ? (
        <span className='text-muted-foreground min-w-0 truncate text-xs'>
          {summary.conditions.join(' · ')}
        </span>
      ) : (
        <span className='text-muted-foreground truncate text-xs'>
          {t('No conditions set — this rule will not match any errors')}
        </span>
      )}
    </div>
  )
}

function RuleItem({
  rule,
  index,
  channelId,
  onUpdate,
  onRemove,
}: RuleItemProps) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(true)
  const [recentErrorsOpen, setRecentErrorsOpen] = useState(false)

  const updateStatusCodes = (values: string[]) => {
    onUpdate(index, { status_codes: normalizeStatusCodes(values) })
  }
  const updateErrorCodes = (values: string[]) => {
    onUpdate(index, { error_codes: normalizeStringList(values) })
  }
  const updateMessages = (values: string[]) => {
    onUpdate(index, { message_contains: normalizeStringList(values) })
  }

  return (
    <div className='border-border/70 overflow-hidden rounded-lg border'>
      <div
        className='hover:bg-muted/40 flex min-h-12 cursor-pointer items-center justify-between gap-3 px-3 py-2 transition-colors select-none'
        onClick={() => setExpanded((value) => !value)}
      >
        <div className='flex min-w-0 items-center gap-2'>
          <span className='bg-primary text-primary-foreground flex size-6 shrink-0 items-center justify-center rounded-full text-xs font-medium'>
            {index + 1}
          </span>
          {expanded ? (
            <span className='text-sm font-medium'>
              {t('Rule {{n}}', { n: index + 1 })}
            </span>
          ) : (
            <RuleSummary rule={rule} />
          )}
        </div>
        <div
          className='flex shrink-0 items-center gap-1'
          onClick={(event) => event.stopPropagation()}
        >
          <Button
            type='button'
            variant='ghost'
            size='icon-sm'
            className='text-destructive hover:text-destructive'
            onClick={() => onRemove(index)}
            aria-label={t('Delete rule')}
            title={t('Delete rule')}
          >
            <Trash2 aria-hidden='true' />
          </Button>
          <Button
            type='button'
            variant='ghost'
            size='icon-sm'
            onClick={() => setExpanded((value) => !value)}
            aria-label={expanded ? t('Collapse rule') : t('Expand rule')}
            title={expanded ? t('Collapse rule') : t('Expand rule')}
          >
            {expanded ? (
              <ChevronUp aria-hidden='true' />
            ) : (
              <ChevronDown aria-hidden='true' />
            )}
          </Button>
        </div>
      </div>

      {expanded && (
        <div className='border-border/60 space-y-4 border-t p-3'>
          <section className='space-y-3'>
            <div className='flex flex-wrap items-start justify-between gap-2'>
              <div>
                <h5 className='text-sm font-medium'>
                  {t('Matching conditions')}
                </h5>
                <p className='text-muted-foreground text-xs'>
                  {t(
                    'Different condition types are AND; values within a type are OR.'
                  )}
                </p>
              </div>
              {channelId !== undefined && channelId > 0 && (
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() => setRecentErrorsOpen(true)}
                >
                  <History aria-hidden='true' />
                  {t('Select from error records')}
                </Button>
              )}
            </div>

            <div className='space-y-3'>
              <div className='space-y-1'>
                <Label className='text-xs'>{t('Status Codes')}</Label>
                <TagInput
                  value={rule.status_codes.map(String)}
                  onChange={updateStatusCodes}
                  separators={STATUS_CODE_SEPARATORS}
                  normalize={normalizeStatusTag}
                  placeholder={t('Enter a status code, then press Enter')}
                  className='text-xs'
                />
              </div>

              <div className='grid gap-3 sm:grid-cols-2'>
                <div className='space-y-1'>
                  <Label className='text-xs'>{t('Error Codes')}</Label>
                  <TagInput
                    value={rule.error_codes}
                    onChange={updateErrorCodes}
                    separators={STRING_SEPARATORS}
                    normalize={normalizeStringTag}
                    placeholder={t('e.g. rate_limit_exceeded')}
                    className='text-xs'
                  />
                </div>
                <div className='space-y-1'>
                  <Label className='text-xs'>{t('Message Keywords')}</Label>
                  <TagInput
                    value={rule.message_contains}
                    onChange={updateMessages}
                    separators={STRING_SEPARATORS}
                    normalize={normalizeStringTag}
                    placeholder={t('e.g. rate limit')}
                    className='text-xs'
                  />
                </div>
              </div>
            </div>

            {!hasCondition(rule) && (
              <div className='flex items-start gap-2 rounded-md bg-amber-500/10 px-2 py-1.5 text-xs text-amber-700 dark:text-amber-300'>
                <AlertTriangle
                  className='mt-0.5 size-3.5 shrink-0'
                  aria-hidden='true'
                />
                <span>
                  {t('No conditions set — this rule will not match any errors')}
                </span>
              </div>
            )}
          </section>

          <section className='space-y-2'>
            <Label
              htmlFor={`error-filter-action-${index}`}
              className='text-sm font-medium'
            >
              {t('Action')}
            </Label>
            <Select
              value={rule.action}
              onValueChange={(value: string | null) => {
                if (
                  value === 'retry' ||
                  value === 'rewrite' ||
                  value === 'replace'
                ) {
                  onUpdate(index, { action: value })
                }
              }}
            >
              <SelectTrigger
                id={`error-filter-action-${index}`}
                className='w-full'
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='retry'>
                  {t('Switch channel and retry')}
                </SelectItem>
                <SelectItem value='rewrite'>
                  {t('Rewrite message and preserve status code')}
                </SelectItem>
                <SelectItem value='replace'>
                  {t('Intercept and replace response')}
                </SelectItem>
              </SelectContent>
            </Select>
          </section>

          {rule.action === 'rewrite' && (
            <section className='space-y-1'>
              <Label
                htmlFor={`error-filter-rewrite-${index}`}
                className='text-xs'
              >
                {t('Rewrite Message')}
              </Label>
              <Input
                id={`error-filter-rewrite-${index}`}
                value={rule.rewrite_message}
                onChange={(event) =>
                  onUpdate(index, { rewrite_message: event.target.value })
                }
                placeholder={t('Enter the rewritten error message')}
              />
              <p className='text-muted-foreground text-xs'>
                {t('Leave empty to preserve the original message.')}
              </p>
            </section>
          )}

          {rule.action === 'replace' && (
            <section className='space-y-2'>
              <Label className='text-xs'>{t('Replace response')}</Label>
              <div className='grid gap-3 sm:grid-cols-[minmax(0,7rem)_1fr]'>
                <div className='space-y-1'>
                  <Label
                    htmlFor={`error-filter-replace-status-${index}`}
                    className='text-xs'
                  >
                    {t('Replace Status Code')}
                  </Label>
                  <Input
                    id={`error-filter-replace-status-${index}`}
                    type='number'
                    min={100}
                    max={599}
                    value={rule.replace_status_code}
                    onChange={(event) => {
                      const parsed = Number.parseInt(event.target.value, 10)
                      onUpdate(index, {
                        replace_status_code:
                          Number.isInteger(parsed) &&
                          parsed >= 100 &&
                          parsed <= 599
                            ? parsed
                            : 200,
                      })
                    }}
                  />
                </div>
                <div className='space-y-1'>
                  <Label
                    htmlFor={`error-filter-replace-message-${index}`}
                    className='text-xs'
                  >
                    {t('Replace Message')}
                  </Label>
                  <Input
                    id={`error-filter-replace-message-${index}`}
                    value={rule.replace_message}
                    onChange={(event) =>
                      onUpdate(index, { replace_message: event.target.value })
                    }
                    placeholder={t('Enter the replacement error message')}
                  />
                </div>
              </div>
            </section>
          )}

          {rule.action === 'retry' && (
            <p className='bg-primary/10 text-primary rounded-md px-3 py-2 text-xs'>
              {t('Matching this rule forces a retry on the next channel.')}
            </p>
          )}
        </div>
      )}

      {channelId !== undefined && channelId > 0 && (
        <RecentErrorsDialog
          open={recentErrorsOpen}
          onOpenChange={setRecentErrorsOpen}
          channelId={channelId}
          onApply={(selected) =>
            onUpdate(index, applyRecentErrorsToRule(rule, selected))
          }
        />
      )}
    </div>
  )
}

interface Props {
  form: UseFormReturn<ChannelFormValues>
  channelId?: number
}

export function ErrorFilterRulesEditor(props: Props) {
  const { t } = useTranslation()
  const raw = props.form.watch('error_filter_rules')
  const rules = useMemo(() => parseRules(raw), [raw])

  const setRules = (next: FilterRule[]) => {
    const normalized = next.map((rule) => normalizeRule(rule))
    props.form.setValue('error_filter_rules', JSON.stringify(normalized), {
      shouldDirty: true,
    })
  }

  const updateRule = (index: number, patch: Partial<FilterRule>) => {
    setRules(
      rules.map((rule, i) => (i === index ? { ...rule, ...patch } : rule))
    )
  }

  return (
    <div className='space-y-3'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div className='flex min-w-0 items-start gap-2'>
          <span className='bg-destructive/10 text-destructive mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-md'>
            <Filter className='size-4' aria-hidden='true' />
          </span>
          <div className='min-w-0'>
            <h4 className='text-sm font-medium'>{t('Error Filter Rules')}</h4>
            <p className='text-muted-foreground text-xs'>
              {t('Configure first-match upstream error handling rules.')}
            </p>
          </div>
        </div>
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={() => setRules([...rules, normalizeRule()])}
        >
          <Plus aria-hidden='true' />
          {t('Add Rule')}
        </Button>
      </div>

      {rules.length === 0 ? (
        <p className='text-muted-foreground rounded-md border border-dashed px-3 py-4 text-center text-xs'>
          {t('No rules configured')}
        </p>
      ) : (
        <div className='space-y-2'>
          {rules.map((rule, index) => (
            <RuleItem
              // Rules do not have persisted identifiers; edits are controlled by index.
              // eslint-disable-next-line react/no-array-index-key
              key={index}
              rule={rule}
              index={index}
              channelId={props.channelId}
              onUpdate={updateRule}
              onRemove={(removeIndex) =>
                setRules(rules.filter((_, i) => i !== removeIndex))
              }
            />
          ))}
        </div>
      )}
    </div>
  )
}
