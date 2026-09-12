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
export type RequestBlacklistFormValues = {
  mode: string
  enabledGroups: string[]
  contentWords: string
  domains: string
  blockMessage: string
  blockStatusCode: number
}

type RequestBlacklistDocument = {
  mode?: unknown
  enabled_groups?: unknown
  content_words?: unknown
  domains?: unknown
  block_message?: unknown
  block_status_code?: unknown
}

const emptyRequestBlacklistValues: RequestBlacklistFormValues = {
  mode: 'off',
  enabledGroups: [],
  contentWords: '',
  domains: '',
  blockMessage: '',
  blockStatusCode: 403,
}

function readStringList(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return value.filter((item): item is string => typeof item === 'string')
}

export function requestBlacklistLinesToList(value: string): string[] {
  return value
    .split(/\r?\n/)
    .map((item) => item.trim())
    .filter(Boolean)
}

export function requestBlacklistListToLines(value: unknown): string {
  return readStringList(value).join('\n')
}

export function parseRequestBlacklistDocument(
  raw: string
): RequestBlacklistFormValues {
  if (!raw.trim()) {
    return { ...emptyRequestBlacklistValues, enabledGroups: [] }
  }

  let document: RequestBlacklistDocument
  try {
    const parsed: unknown = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return { ...emptyRequestBlacklistValues, enabledGroups: [] }
    }
    document = parsed as RequestBlacklistDocument
  } catch {
    return { ...emptyRequestBlacklistValues, enabledGroups: [] }
  }

  const rawStatus = document.block_status_code
  const blockStatusCode =
    typeof rawStatus === 'number' && rawStatus !== 0 ? rawStatus : 403

  return {
    mode: typeof document.mode === 'string' ? document.mode : 'off',
    enabledGroups: readStringList(document.enabled_groups),
    contentWords: requestBlacklistListToLines(document.content_words),
    domains: requestBlacklistListToLines(document.domains),
    blockMessage:
      typeof document.block_message === 'string' ? document.block_message : '',
    blockStatusCode,
  }
}

export function serializeRequestBlacklistDocument(
  values: RequestBlacklistFormValues
): string {
  return JSON.stringify({
    mode: values.mode,
    enabled_groups: values.enabledGroups,
    content_words: requestBlacklistLinesToList(values.contentWords),
    domains: requestBlacklistLinesToList(values.domains),
    block_message: values.blockMessage,
    block_status_code: values.blockStatusCode,
  })
}
