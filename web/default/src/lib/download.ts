/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
export function downloadTextFile(text: string, filename: string): void {
  const sanitizedFilename = [...filename]
    .filter((character) => {
      if (character === '/' || character === '\\') return false
      const code = character.charCodeAt(0)
      return code > 0x1f && code !== 0x7f && (code < 0x80 || code > 0x9f)
    })
    .join('')
    .trim()
  const resolvedFilename = sanitizedFilename || 'download.txt'
  const blob = new Blob([text], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  try {
    const link = document.createElement('a')
    link.href = url
    link.download = resolvedFilename
    link.click()
  } finally {
    // Revoking synchronously can cancel a download that has not started
    // reading the blob yet, so release the URL on the next task instead.
    setTimeout(() => URL.revokeObjectURL(url), 0)
  }
}
