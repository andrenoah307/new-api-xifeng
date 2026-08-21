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
import type { ReactNode } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider } from 'react-i18next'

const bunTestModule: string = 'bun:test'
const { mock } = await import(bunTestModule)

mock.module('@/components/rich-content', () => ({
  RichContent: (props: { content: string }) => <div>{props.content}</div>,
}))

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  interpolation: { escapeValue: false },
})

function renderContent(node: ReactNode): string {
  return renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>{node}</I18nextProvider>
  )
}

test('renders notice content', async () => {
  const { NoticeContent } = await import('./notification-content')
  const markup = renderContent(
    <NoticeContent notice='Scheduled maintenance' loading={false} />
  )

  assert.match(markup, /Scheduled maintenance/)
  assert.doesNotMatch(markup, /No announcements at this time/)
})

test('renders the notice empty state', async () => {
  const { NoticeContent } = await import('./notification-content')
  const markup = renderContent(<NoticeContent notice='' loading={false} />)

  assert.match(markup, /No announcements at this time/)
})

test('renders announcement content and extra details', async () => {
  const { AnnouncementsContent } = await import('./notification-content')
  const markup = renderContent(
    <AnnouncementsContent
      announcements={[
        { id: 1, content: 'Platform update', extra: 'Read the release notes' },
      ]}
      loading={false}
    />
  )

  assert.match(markup, /Platform update/)
  assert.match(markup, /Read the release notes/)
  assert.doesNotMatch(markup, /No system announcements/)
})

test('keeps notice content available when announcements are empty', async () => {
  const { AnnouncementsContent, NoticeContent } = await import(
    './notification-content'
  )
  const markup = renderContent(
    <>
      <NoticeContent notice='Important notice' loading={false} />
      <AnnouncementsContent announcements={[]} loading={false} />
    </>
  )

  assert.match(markup, /Important notice/)
  assert.match(markup, /No system announcements/)
})
