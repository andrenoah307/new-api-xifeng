import { useEffect, useState } from 'react'
import { useForm, useFieldArray } from 'react-hook-form'
import { useQuery } from '@tanstack/react-query'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Plus, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
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
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { createRule, updateRule, getRule, getGroupOptions } from '../api'
import type { GroupOption } from '../api'
import { METRICS, METRICS_MAP, OPS, STRING_OPS, STRING_METRICS, SUCCESS_MESSAGES } from '../constants'
import type { AutoGroupRule, AutoGroupCondition } from '../types'
import { useAutoGroup } from './auto-group-provider'

// ============================================================================
// Form Schema
// ============================================================================

const conditionSchema = z.object({
  metric: z.string().min(1, 'Metric is required'),
  op: z.string().min(1, 'Operator is required'),
  value: z.number(),
  value_str: z.string().optional(),
  param: z.number().optional(),
})

const formSchema = z.object({
  name: z.string().min(1, 'Name is required').max(100),
  description: z.string().optional().or(z.literal('')),
  enabled: z.boolean(),
  priority: z.number().min(0),
  target_group: z.string().min(1, 'Target group is required'),
  match_mode: z.enum(['all', 'any']),
  conditions: z.array(conditionSchema).min(1, 'At least one condition is required'),
})

type FormValues = z.infer<typeof formSchema>

const DEFAULT_VALUES: FormValues = {
  name: '',
  description: '',
  enabled: true,
  priority: 0,
  target_group: '',
  match_mode: 'all',
  conditions: [{ metric: 'total_spend', op: '>=', value: 0 }],
}

const DEFAULT_CONDITION = { metric: 'total_spend', op: '>=', value: 0 }

// ============================================================================
// Component
// ============================================================================

type RuleMutateDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: AutoGroupRule
}

export function RuleMutateDrawer({
  open,
  onOpenChange,
  currentRow,
}: RuleMutateDrawerProps) {
  const { t } = useTranslation()
  const isUpdate = !!currentRow
  const { triggerRefresh } = useAutoGroup()
  const [isSubmitting, setIsSubmitting] = useState(false)

  const { data: groupOptions = [] } = useQuery({
    queryKey: ['groups-with-desc'],
    queryFn: getGroupOptions,
  })

  const form = useForm<FormValues>({
    resolver: zodResolver(formSchema),
    defaultValues: DEFAULT_VALUES,
  })

  const { fields, append, remove } = useFieldArray({
    control: form.control,
    name: 'conditions',
  })

  useEffect(() => {
    if (open && isUpdate && currentRow) {
      getRule(currentRow.id).then((result) => {
        if (result.success && result.data) {
          const rule = result.data
          let conditions: AutoGroupCondition[] = []
          try {
            conditions = JSON.parse(rule.conditions)
          } catch {
            conditions = [DEFAULT_CONDITION]
          }
          form.reset({
            name: rule.name,
            description: rule.description,
            enabled: rule.enabled,
            priority: rule.priority,
            target_group: rule.target_group,
            match_mode: rule.match_mode,
            conditions:
              conditions.length > 0 ? conditions : [DEFAULT_CONDITION],
          })
        }
      }).catch(() => {
        // 拉取规则失败：错误提示由拦截器统一弹出
      })
    } else if (open && !isUpdate) {
      form.reset(DEFAULT_VALUES)
    }
  }, [open, isUpdate, currentRow, form])

  const onSubmit = async (data: FormValues) => {
    setIsSubmitting(true)
    try {
      if (isUpdate && currentRow) {
        const result = await updateRule({
          ...data,
          id: currentRow.id,
          description: data.description || '',
        })
        if (result.success) {
          toast.success(t(SUCCESS_MESSAGES.RULE_UPDATED))
          onOpenChange(false)
          triggerRefresh()
        }
      } else {
        const result = await createRule({
          ...data,
          description: data.description || '',
        })
        if (result.success) {
          toast.success(t(SUCCESS_MESSAGES.RULE_CREATED))
          onOpenChange(false)
          triggerRefresh()
        }
      }
    } catch {
      // HTTP 层错误：拦截器已提示，保持表单打开
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Sheet
      open={open}
      onOpenChange={(v) => {
        onOpenChange(v)
        if (!v) form.reset()
      }}
    >
      <SheetContent className='flex h-dvh w-full flex-col gap-0 overflow-hidden p-0 sm:max-w-[600px]'>
        <SheetHeader className='border-b px-4 py-3 text-start sm:px-6 sm:py-4'>
          <SheetTitle>
            {isUpdate ? t('Edit Rule') : t('Create Rule')}
          </SheetTitle>
          <SheetDescription>
            {isUpdate
              ? t('Update the auto group rule.')
              : t('Create a new auto group rule.')}{' '}
            {t("Click save when you're done.")}
          </SheetDescription>
        </SheetHeader>
        <Form {...form}>
          <form
            id='auto-group-rule-form'
            onSubmit={form.handleSubmit(onSubmit)}
            className='flex-1 space-y-4 overflow-y-auto px-3 py-3 pb-4 sm:space-y-6 sm:px-4'
          >
            <FormField
              control={form.control}
              name='name'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Name')}</FormLabel>
                  <FormControl>
                    <Input {...field} placeholder={t('Enter rule name')} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='description'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Description')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      placeholder={t('Optional description')}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className='grid grid-cols-2 gap-4'>
              <FormField
                control={form.control}
                name='priority'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Priority')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        type='number'
                        min='0'
                        placeholder='0'
                        onChange={(e) =>
                          field.onChange(parseInt(e.target.value, 10) || 0)
                        }
                      />
                    </FormControl>
                    <FormDescription>{t('Higher = evaluated first')}</FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='enabled'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Enabled')}</FormLabel>
                    <FormControl>
                      <div className='pt-2'>
                        <Switch
                          checked={field.value}
                          onCheckedChange={field.onChange}
                        />
                      </div>
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <FormField
              control={form.control}
              name='target_group'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Target Group')}</FormLabel>
                  <Select value={field.value} onValueChange={field.onChange}>
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue placeholder={t('Select target group')} />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      {groupOptions.map((g) => (
                        <SelectItem key={g.value} value={g.value}>
                          <span className='flex items-center gap-2'>
                            <span>{g.value}</span>
                            {g.desc !== g.value && (
                              <span className='text-muted-foreground text-xs'>
                                {g.desc}
                              </span>
                            )}
                          </span>
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <FormDescription>
                    {t('The group users will be assigned to when rule matches')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='match_mode'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Match Mode')}</FormLabel>
                  <Select
                    value={field.value}
                    onValueChange={field.onChange}
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      <SelectItem value='all'>{t('Match All (AND)')}</SelectItem>
                      <SelectItem value='any'>{t('Match Any (OR)')}</SelectItem>
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className='space-y-3'>
              <div className='flex items-center justify-between'>
                <FormLabel>{t('Conditions')}</FormLabel>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() => append(DEFAULT_CONDITION)}
                >
                  <Plus className='mr-1 h-3 w-3' />
                  {t('Add Condition')}
                </Button>
              </div>

              {fields.map((field, index) => {
                const metricValue = form.watch(`conditions.${index}.metric`)
                const metricConfig = METRICS_MAP[metricValue]
                const showParam = metricConfig?.needsParam
                const isStringMetric = STRING_METRICS.has(metricValue)

                return (
                  <div
                    key={field.id}
                    className='rounded-md border p-3 space-y-3'
                  >
                    <div className='flex items-start gap-2'>
                      <div className='flex-1 grid grid-cols-2 gap-2 sm:grid-cols-3'>
                        <FormField
                          control={form.control}
                          name={`conditions.${index}.metric`}
                          render={({ field: f }) => (
                            <FormItem className='col-span-2 sm:col-span-1'>
                              <Select
                                value={f.value}
                                onValueChange={(v) => {
                                  const wasString = STRING_METRICS.has(f.value)
                                  const nowString = STRING_METRICS.has(v)
                                  f.onChange(v)
                                  if (wasString !== nowString) {
                                    if (nowString) {
                                      form.setValue(`conditions.${index}.op`, '==')
                                      form.setValue(`conditions.${index}.value`, 0)
                                      form.setValue(`conditions.${index}.value_str`, '')
                                    } else {
                                      form.setValue(`conditions.${index}.value_str`, undefined)
                                    }
                                  }
                                }}
                              >
                                <FormControl>
                                  <SelectTrigger>
                                    <SelectValue
                                      placeholder={t('Metric')}
                                    />
                                  </SelectTrigger>
                                </FormControl>
                                <SelectContent>
                                  {METRICS.map((m) => (
                                    <SelectItem key={m.key} value={m.key}>
                                      {t(m.labelKey)}
                                    </SelectItem>
                                  ))}
                                </SelectContent>
                              </Select>
                              <FormMessage />
                            </FormItem>
                          )}
                        />

                        <FormField
                          control={form.control}
                          name={`conditions.${index}.op`}
                          render={({ field: f }) => (
                            <FormItem>
                              <Select
                                value={f.value}
                                onValueChange={f.onChange}
                              >
                                <FormControl>
                                  <SelectTrigger>
                                    <SelectValue placeholder={t('Op')} />
                                  </SelectTrigger>
                                </FormControl>
                                <SelectContent>
                                  {(isStringMetric ? STRING_OPS : OPS).map((op) => (
                                    <SelectItem
                                      key={op.value}
                                      value={op.value}
                                    >
                                      {op.label}
                                    </SelectItem>
                                  ))}
                                </SelectContent>
                              </Select>
                              <FormMessage />
                            </FormItem>
                          )}
                        />

                        {isStringMetric ? (
                          <FormField
                            control={form.control}
                            name={`conditions.${index}.value_str`}
                            render={({ field: f }) => (
                              <FormItem>
                                <Select
                                  value={f.value || ''}
                                  onValueChange={f.onChange}
                                >
                                  <FormControl>
                                    <SelectTrigger>
                                      <SelectValue
                                        placeholder={t('Select group')}
                                      />
                                    </SelectTrigger>
                                  </FormControl>
                                  <SelectContent>
                                    {groupOptions.map((g) => (
                                      <SelectItem key={g.value} value={g.value}>
                                        <span className='flex items-center gap-2'>
                                          <span>{g.value}</span>
                                          {g.desc !== g.value && (
                                            <span className='text-muted-foreground text-xs'>
                                              {g.desc}
                                            </span>
                                          )}
                                        </span>
                                      </SelectItem>
                                    ))}
                                  </SelectContent>
                                </Select>
                                <FormMessage />
                              </FormItem>
                            )}
                          />
                        ) : (
                          <FormField
                            control={form.control}
                            name={`conditions.${index}.value`}
                            render={({ field: f }) => (
                              <FormItem>
                                <FormControl>
                                  <Input
                                    {...f}
                                    type='number'
                                    step='any'
                                    placeholder={t('Value')}
                                    onChange={(e) =>
                                      f.onChange(
                                        parseFloat(e.target.value) || 0
                                      )
                                    }
                                  />
                                </FormControl>
                                <FormMessage />
                              </FormItem>
                            )}
                          />
                        )}
                      </div>

                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        className='mt-0.5 h-8 w-8 shrink-0 p-0 text-destructive hover:text-destructive'
                        onClick={() => {
                          if (fields.length > 1) remove(index)
                        }}
                        disabled={fields.length <= 1}
                      >
                        <Trash2 className='h-4 w-4' />
                      </Button>
                    </div>

                    {showParam && !isStringMetric && (
                      <FormField
                        control={form.control}
                        name={`conditions.${index}.param`}
                        render={({ field: f }) => (
                          <FormItem>
                            <FormLabel className='text-xs'>
                              {t('Hours (N)')}
                            </FormLabel>
                            <FormControl>
                              <Input
                                value={f.value ?? ''}
                                type='number'
                                min='1'
                                placeholder={t('e.g. 24')}
                                onChange={(e) =>
                                  f.onChange(
                                    parseInt(e.target.value, 10) || undefined
                                  )
                                }
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    )}
                  </div>
                )
              })}
              {form.formState.errors.conditions?.root && (
                <p className='text-sm text-destructive'>
                  {form.formState.errors.conditions.root.message}
                </p>
              )}
            </div>
          </form>
        </Form>
        <SheetFooter className='grid grid-cols-2 gap-2 border-t px-4 py-3 sm:flex sm:px-6 sm:py-4'>
          <SheetClose render={<Button variant='outline' />}>
            {t('Close')}
          </SheetClose>
          <Button
            form='auto-group-rule-form'
            type='submit'
            disabled={isSubmitting}
          >
            {isSubmitting ? t('Saving...') : t('Save changes')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
