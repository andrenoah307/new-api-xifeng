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
import { useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'

import type { AnnouncementItem } from '@/components/notification-content'
import { useStatus } from '@/hooks/use-status'
import { getNotice } from '@/lib/api'
import { useNotificationStore } from '@/stores/notification-store'

function hashString(input: string): string {
  let hash = 0
  if (!input) return '0'

  for (let i = 0; i < input.length; i += 1) {
    const chr = input.charCodeAt(i)
    hash = (hash << 5) - hash + chr
    hash |= 0
  }

  return hash.toString(36)
}

/**
 * Generate a unique key for an announcement
 * Prefer backend id, fall back to a content hash so edits register
 */
function getAnnouncementKey(item: AnnouncementItem): string {
  if (!item) return ''

  if (item.id !== undefined && item.id !== null) {
    return `id:${item.id}`
  }

  const fingerprint = JSON.stringify({
    publishDate: item.publishDate || '',
    content: (item.content || '').trim(),
    extra: (item.extra || '').trim(),
    type: item.type || '',
    title: (item.title || '').trim(),
    link: (item.link || '').trim(),
  })
  return `hash:${hashString(fingerprint)}`
}

/**
 * Hook to manage notifications (Notice + Announcements)
 * Provides unread counts and read status management
 */
export function useNotifications() {
  const [popoverOpen, setPopoverOpen] = useState(false)
  const [activeTab, setActiveTab] = useState<'notice' | 'announcements'>(
    'notice'
  )

  const noticeData = useNoticeData()

  // Notification store
  const {
    lastReadNotice,
    markNoticeRead,
    markAnnouncementsRead,
    isAnnouncementRead,
  } = useNotificationStore()

  // Calculate unread counts
  const unreadCounts = useMemo(() => {
    const noticeUnread =
      noticeData.notice && noticeData.notice !== lastReadNotice ? 1 : 0

    const announcementsUnread = noticeData.announcements.filter((item) => {
      const key = getAnnouncementKey(item)
      return !isAnnouncementRead(key)
    }).length

    return {
      notice: noticeUnread,
      announcements: announcementsUnread,
      total: noticeUnread + announcementsUnread,
    }
  }, [
    noticeData.notice,
    lastReadNotice,
    noticeData.announcements,
    isAnnouncementRead,
  ])

  const markAnnouncementsAsRead = () => {
    if (noticeData.announcements.length > 0) {
      const allKeys = noticeData.announcements.map((item) =>
        getAnnouncementKey(item)
      )
      markAnnouncementsRead(allKeys)
    }
  }

  // Handle popover open
  const handleOpenPopover = (tab?: 'notice' | 'announcements') => {
    const nextTab = tab || activeTab

    // Mark currently visible content as read when opening the notification center
    if (noticeData.notice) {
      markNoticeRead(noticeData.notice)
    }
    if (nextTab === 'announcements') {
      markAnnouncementsAsRead()
    }

    setActiveTab(nextTab)
    setPopoverOpen(true)
  }

  const handlePopoverOpenChange = (open: boolean) => {
    if (open) {
      handleOpenPopover(activeTab)
      return
    }

    setPopoverOpen(false)
  }

  // Handle tab change - mark announcements as read when switching to that tab
  const handleTabChange = (tab: 'notice' | 'announcements') => {
    setActiveTab(tab)

    if (tab === 'announcements') {
      markAnnouncementsAsRead()
    }
  }

  return {
    // Data
    notice: noticeData.notice,
    announcements: noticeData.announcements,
    loading: noticeData.noticeLoading || noticeData.announcementsLoading,

    // Unread counts
    unreadCount: unreadCounts.total,
    unreadNoticeCount: unreadCounts.notice,
    unreadAnnouncementsCount: unreadCounts.announcements,

    // Popover state
    popoverOpen,
    setPopoverOpen: handlePopoverOpenChange,
    activeTab,
    setActiveTab: handleTabChange,

    // Actions
    openPopover: handleOpenPopover,
    closePopover: () => setPopoverOpen(false),
    refetchNotice: noticeData.refetchNotice,
  }
}

export function useNoticeData() {
  const noticeQuery = useQuery({
    queryKey: ['notice'],
    queryFn: getNotice,
    staleTime: 1000 * 60 * 5, // 5 minutes
  })

  const { status, loading: statusLoading } = useStatus()
  const announcementsEnabled = status?.announcements_enabled ?? false
  const statusAnnouncements = status?.announcements
  const announcements = useMemo<AnnouncementItem[]>(() => {
    return announcementsEnabled
      ? ((statusAnnouncements || []) as AnnouncementItem[]).slice(0, 20)
      : []
  }, [announcementsEnabled, statusAnnouncements])

  const noticeContent = noticeQuery.data?.success
    ? (noticeQuery.data.data || '').trim()
    : ''

  return {
    notice: noticeContent,
    announcements,
    noticeLoading: noticeQuery.isLoading,
    announcementsLoading: statusLoading,
    refetchNotice: noticeQuery.refetch,
  }
}
