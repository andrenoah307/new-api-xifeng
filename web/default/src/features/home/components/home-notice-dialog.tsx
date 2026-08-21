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
import { useEffect, useRef, useState } from 'react'

import { useCnDisclaimerBlockingStore } from '@/features/cn-disclaimer/lib/blocking-store'
import { useNoticeData } from '@/hooks/use-notifications'
import { useNotificationStore } from '@/stores/notification-store'

import {
  closeNoticeForToday,
  shouldAutoOpenNotice,
} from '../lib/notice-auto-open'
import { NoticeDialog } from './notice-dialog'

export function HomeNoticeDialog() {
  const noticeData = useNoticeData()
  const [open, setOpen] = useState(false)
  const hasAutoOpened = useRef(false)
  const noticeClosed = useNotificationStore((state) =>
    state.isNoticeClosed()
  )
  const setClosedUntilDate = useNotificationStore(
    (state) => state.setClosedUntilDate
  )
  const blockedByModal = useCnDisclaimerBlockingStore(
    (state) => state.blocking
  )

  useEffect(() => {
    if (hasAutoOpened.current) return
    if (
      !shouldAutoOpenNotice({
        notice: noticeData.notice,
        noticeClosed,
        blockedByModal,
      })
    ) {
      return
    }

    hasAutoOpened.current = true
    setOpen(true)
  }, [blockedByModal, noticeClosed, noticeData.notice])

  const handleCloseForToday = () => {
    closeNoticeForToday({
      setClosedUntilDate,
      close: () => setOpen(false),
    })
  }

  return (
    <NoticeDialog
      open={open}
      onOpenChange={setOpen}
      onCloseForToday={handleCloseForToday}
      notice={noticeData.notice}
      announcements={noticeData.announcements}
      noticeLoading={noticeData.noticeLoading}
      announcementsLoading={noticeData.announcementsLoading}
    />
  )
}
