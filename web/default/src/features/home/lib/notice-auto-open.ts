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
interface ShouldAutoOpenNoticeOptions {
  notice: string
  noticeClosed: boolean
  blockedByModal: boolean
}

interface CloseNoticeForTodayOptions {
  setClosedUntilDate: (date: string) => void
  close: () => void
  getCurrentDate?: () => Date
}

export function shouldAutoOpenNotice(
  options: ShouldAutoOpenNoticeOptions
): boolean {
  return (
    options.notice.trim() !== '' &&
    !options.noticeClosed &&
    !options.blockedByModal
  )
}

export function closeNoticeForToday(
  options: CloseNoticeForTodayOptions
): void {
  try {
    const currentDate = options.getCurrentDate?.() ?? new Date()
    options.setClosedUntilDate(currentDate.toDateString())
  } finally {
    options.close()
  }
}
