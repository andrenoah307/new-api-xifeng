import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Plus, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'

interface MapPatternsRow {
  key: string
  patterns: string
}

/**
 * 可视化 + JSON 双模编辑 `Record<string, string[]>` 形状的 JSON 字符串
 * （如地区黑名单 {"CN": ["default"], "RU": ["*"]}）。
 * 值始终以 JSON 字符串对外（受控），空 map 输出空串，兼容现有 option 保存逻辑。
 */
export function MapPatternsField({
  value,
  onChange,
  keyPlaceholder,
  patternsPlaceholder,
  jsonPlaceholder,
}: {
  value: string
  onChange: (value: string) => void
  keyPlaceholder: string
  patternsPlaceholder: string
  jsonPlaceholder: string
}) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<'visual' | 'json'>('visual')
  // 可视化模式的行内草稿：让"CN, RU"这类逗号输入不被序列化立即打断
  const [draftRows, setDraftRows] = useState<MapPatternsRow[] | null>(null)

  const parsed = useMemo(() => {
    if (!value.trim()) return { rows: [] as MapPatternsRow[], valid: true }
    try {
      const obj = JSON.parse(value) as unknown
      if (typeof obj !== 'object' || obj === null || Array.isArray(obj)) {
        return { rows: [], valid: false }
      }
      const rows = Object.entries(obj as Record<string, unknown>).map(
        ([key, patterns]) => ({
          key,
          patterns: Array.isArray(patterns) ? patterns.join(', ') : '',
        })
      )
      return { rows, valid: true }
    } catch {
      return { rows: [], valid: false }
    }
  }, [value])

  const rows = draftRows ?? parsed.rows

  const commitRows = (next: MapPatternsRow[]) => {
    setDraftRows(next)
    const obj: Record<string, string[]> = {}
    for (const row of next) {
      const key = row.key.trim()
      if (!key) continue
      const patterns = row.patterns
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean)
      obj[key] = patterns
    }
    onChange(Object.keys(obj).length > 0 ? JSON.stringify(obj) : '')
  }

  const switchMode = (next: string) => {
    if (next !== 'visual' && next !== 'json') return
    setDraftRows(null)
    setMode(next)
  }

  return (
    <div className='space-y-2'>
      <Tabs value={mode} onValueChange={switchMode}>
        <TabsList>
          <TabsTrigger value='visual' disabled={!parsed.valid}>
            {t('Visual')}
          </TabsTrigger>
          <TabsTrigger value='json'>JSON</TabsTrigger>
        </TabsList>
      </Tabs>
      {mode === 'json' || !parsed.valid ? (
        <>
          {!parsed.valid && (
            <p className='text-destructive text-xs'>
              {t('Invalid JSON — fix it here before switching to visual mode')}
            </p>
          )}
          <Textarea
            rows={6}
            placeholder={jsonPlaceholder}
            className='font-mono text-sm'
            value={value}
            onChange={(e) => onChange(e.target.value)}
          />
        </>
      ) : (
        <div
          className='space-y-2'
          // 行内输入回车不应提交外层设置表单
          onKeyDown={(e) => {
            if (
              e.key === 'Enter' &&
              (e.target as HTMLElement).tagName === 'INPUT'
            ) {
              e.preventDefault()
            }
          }}
        >
          {rows.map((row, i) => (
            <div key={i} className='flex items-center gap-2'>
              <Input
                value={row.key}
                placeholder={keyPlaceholder}
                className='w-28 font-mono text-sm'
                onChange={(e) =>
                  commitRows(
                    rows.map((r, idx) =>
                      idx === i ? { ...r, key: e.target.value } : r
                    )
                  )
                }
              />
              <Input
                value={row.patterns}
                placeholder={patternsPlaceholder}
                className='flex-1 font-mono text-sm'
                onChange={(e) =>
                  commitRows(
                    rows.map((r, idx) =>
                      idx === i ? { ...r, patterns: e.target.value } : r
                    )
                  )
                }
              />
              <Button
                type='button'
                variant='ghost'
                size='icon'
                className='h-8 w-8 shrink-0'
                onClick={() => commitRows(rows.filter((_, idx) => idx !== i))}
              >
                <X className='h-3.5 w-3.5' />
              </Button>
            </div>
          ))}
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => commitRows([...rows, { key: '', patterns: '' }])}
          >
            <Plus className='mr-1 h-3.5 w-3.5' />
            {t('Add Entry')}
          </Button>
        </div>
      )}
    </div>
  )
}
