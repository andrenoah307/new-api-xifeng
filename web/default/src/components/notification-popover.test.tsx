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
import assert from 'node:assert/strict'
import { test } from 'node:test'

import { createInstance } from 'i18next'
import type { ReactElement, ReactNode } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider } from 'react-i18next'

interface MockContainerProps {
  children?: ReactNode
  render?: ReactElement
}

function MockContainer(props: MockContainerProps) {
  return (
    <div>
      {props.render}
      {props.children}
    </div>
  )
}

const bunTestModule: string = 'bun:test'
const { mock } = await import(bunTestModule)

mock.module('@/components/ui/popover', () => ({
  Popover: MockContainer,
  PopoverContent: MockContainer,
  PopoverHeader: MockContainer,
  PopoverTitle: MockContainer,
  PopoverTrigger: MockContainer,
}))

mock.module('@/components/ui/tabs', () => ({
  Tabs: MockContainer,
  TabsContent: MockContainer,
  TabsList: MockContainer,
  TabsTrigger: MockContainer,
}))

mock.module('@/components/rich-content', () => ({
  RichContent: (props: { content: string }) => <div>{props.content}</div>,
}))

const { NotificationPopover } = await import('./notification-popover')

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  interpolation: { escapeValue: false },
})

test('notification popover keeps its visible notification contract', () => {
  const markup = renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <NotificationPopover
        open
        onOpenChange={() => {}}
        unreadCount={120}
        activeTab='notice'
        onTabChange={() => {}}
        notice='Scheduled maintenance notice'
        announcements={[{ id: 1, content: 'Timeline announcement' }]}
        loading={false}
      />
    </I18nextProvider>
  )

  assert.match(markup, /aria-label="Notifications"/)
  assert.match(markup, />99\+<\/span>/)
  assert.match(markup, /System Announcements/)
  assert.match(markup, />Notice<\/div>/)
  assert.match(markup, />Timeline<\/div>/)
  assert.match(markup, /Scheduled maintenance notice/)
  assert.match(markup, /Timeline announcement/)
  assert.match(markup, />Close<\/button>/)
})
