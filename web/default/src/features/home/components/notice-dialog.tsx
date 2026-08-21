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
import { Bell, Megaphone } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import {
  AnnouncementsContent,
  type AnnouncementItem,
  NoticeContent,
} from '@/components/notification-content'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

interface NoticeDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCloseForToday: () => void
  notice: string
  announcements: AnnouncementItem[]
  noticeLoading: boolean
  announcementsLoading: boolean
}

export function NoticeDialog(props: NoticeDialogProps) {
  const { t } = useTranslation()

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='flex max-h-[calc(100svh-2rem)] min-h-0 flex-col overflow-hidden sm:max-w-2xl [&>button.absolute]:hidden'>
        <DialogHeader className='shrink-0'>
          <DialogTitle>{t('System Announcements')}</DialogTitle>
          <DialogDescription>
            {t('Latest platform updates and notices')}
          </DialogDescription>
        </DialogHeader>

        <Tabs defaultValue='notice' className='min-h-0 flex-1'>
          <TabsList className='grid w-full shrink-0 grid-cols-2'>
            <TabsTrigger value='notice' className='gap-1.5'>
              <Bell className='size-3.5' />
              {t('Notice')}
            </TabsTrigger>
            <TabsTrigger value='announcements' className='gap-1.5'>
              <Megaphone className='size-3.5' />
              {t('Timeline')}
            </TabsTrigger>
          </TabsList>

          <TabsContent
            value='notice'
            className='mt-2 min-h-0 flex-1 overflow-hidden'
          >
            <NoticeContent
              notice={props.notice}
              loading={props.noticeLoading}
            />
          </TabsContent>

          <TabsContent
            value='announcements'
            className='mt-2 min-h-0 flex-1 overflow-hidden'
          >
            <AnnouncementsContent
              announcements={props.announcements}
              loading={props.announcementsLoading}
            />
          </TabsContent>
        </Tabs>

        <DialogFooter className='shrink-0'>
          <Button
            type='button'
            variant='outline'
            onClick={props.onCloseForToday}
          >
            {t('Close for today')}
          </Button>
          <DialogClose render={<Button type='button' />}>
            {t('Close')}
          </DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
