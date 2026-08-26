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
import { X } from 'lucide-react'
import { useState, useRef, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

export interface TagInputProps {
  value: string[]
  onChange: (tags: string[]) => void
  placeholder?: string
  className?: string
  disabled?: boolean
  separators?: string[]
  normalize?: (raw: string) => string | null
}

const DEFAULT_SEPARATORS = [',']

export function TagInput({
  value = [],
  onChange,
  placeholder,
  className,
  disabled = false,
  separators = DEFAULT_SEPARATORS,
  normalize,
}: TagInputProps) {
  const { t } = useTranslation()
  const placeholderText = placeholder ?? t('Add tags...')
  const [inputValue, setInputValue] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)
  const isComposingRef = useRef(false)
  const activeSeparators = separators ?? DEFAULT_SEPARATORS

  const addTags = (rawInput: string) => {
    const rawTags = activeSeparators.reduce<string[]>(
      (parts, separator) => {
        if (!separator) return parts
        return parts.flatMap((part) => part.split(separator))
      },
      [rawInput]
    )
    const nextValue = [...value]

    for (const rawTag of rawTags) {
      if (!rawTag.trim()) continue
      const normalized = normalize ? normalize(rawTag) : rawTag.trim()
      const tag = normalized?.trim() ?? ''
      if (tag && !nextValue.includes(tag)) {
        nextValue.push(tag)
      }
    }

    if (nextValue.length !== value.length) {
      onChange(nextValue)
    }
    setInputValue('')
  }

  const removeTag = (tagToRemove: string) => {
    onChange(value.filter((tag) => tag !== tagToRemove))
  }

  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    const nativeEvent = e.nativeEvent
    const isComposing =
      isComposingRef.current ||
      nativeEvent?.isComposing ||
      nativeEvent?.keyCode === 229 ||
      e.keyCode === 229

    if (isComposing) return

    if (e.key === 'Enter') {
      e.preventDefault()
      addTags(inputValue)
    } else if (activeSeparators.includes(e.key)) {
      e.preventDefault()
      addTags(inputValue)
    } else if (e.key === 'Backspace' && !inputValue && value.length > 0) {
      const lastTag = value.at(-1)
      if (lastTag !== undefined) removeTag(lastTag)
    }
  }

  const handleBlur = () => {
    if (inputValue.trim()) {
      addTags(inputValue)
    }
  }

  return (
    <div
      className={cn(
        'border-input focus-within:border-ring focus-within:ring-ring/50 flex min-h-9 w-full flex-wrap items-center gap-2 rounded-md border bg-transparent px-3 py-2 text-base shadow-xs transition-[color,box-shadow] outline-none focus-within:ring-[3px] disabled:cursor-not-allowed disabled:opacity-50 md:text-sm',
        className
      )}
      onClick={() => inputRef.current?.focus()}
    >
      {value.map((tag) => (
        <Badge key={tag} variant='secondary' className='gap-1 pr-1'>
          {tag}
          {!disabled && (
            <Button
              type='button'
              variant='ghost'
              size='icon-sm'
              aria-label='Remove tag'
              onClick={(e) => {
                e.stopPropagation()
                removeTag(tag)
              }}
              className='hover:bg-secondary-foreground/20 size-auto rounded-sm p-0'
            >
              <X className='h-3 w-3' aria-hidden='true' />
            </Button>
          )}
        </Badge>
      ))}
      <input
        ref={inputRef}
        type='text'
        value={inputValue}
        onChange={(e) => setInputValue(e.target.value)}
        onKeyDown={handleKeyDown}
        onCompositionStart={() => {
          isComposingRef.current = true
        }}
        onCompositionEnd={() => {
          isComposingRef.current = false
        }}
        onBlur={handleBlur}
        placeholder={value.length === 0 ? placeholderText : ''}
        disabled={disabled}
        className='placeholder:text-muted-foreground min-w-[120px] flex-1 border-0 bg-transparent shadow-none outline-none focus-visible:ring-0'
      />
    </div>
  )
}
