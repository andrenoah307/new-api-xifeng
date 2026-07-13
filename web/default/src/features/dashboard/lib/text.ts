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
import { normalizeInterfaceLanguage } from '@/i18n/languages'

/**
 * Resolve announcement content/extra for the current interface language.
 * Falls back to the default `content`/`extra` when no non-blank translation
 * exists for the language — legacy announcements without `contentI18n`
 * therefore render unchanged.
 */
export function resolveAnnouncementText(
  item: {
    content: string
    extra?: string
    contentI18n?: Record<string, string>
    extraI18n?: Record<string, string>
  },
  language?: string | null
): { content: string; extra?: string } {
  const lang = normalizeInterfaceLanguage(language)
  const content = item.contentI18n?.[lang]?.trim()
    ? item.contentI18n[lang]
    : item.content
  const extra = item.extraI18n?.[lang]?.trim() ? item.extraI18n[lang] : item.extra
  return { content, extra }
}

/**
 * Get plain text preview (strip HTML tags and Markdown formatting)
 */
export function getPreviewText(
  content: string,
  maxLength: number = 60
): string {
  if (!content) return ''
  const plainText = content
    .replace(/<[^>]*>/g, '') // Remove HTML tags
    .replace(/[#*_]/g, '') // Remove Markdown formatting symbols
    .trim()
  return plainText.length > maxLength
    ? plainText.substring(0, maxLength) + '...'
    : plainText
}
