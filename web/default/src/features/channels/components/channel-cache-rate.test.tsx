import assert from 'node:assert/strict'
import { test } from 'node:test'

import { createInstance } from 'i18next'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider } from 'react-i18next'

import { ChannelCacheRate } from './channel-cache-rate'

test('enabled channel cache rates distinguish zero, low rates and no data', async () => {
  const i18n = createInstance()
  await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })
  for (const [rate, expected] of [
    [0, '0.0%'],
    [1.2, '1.2%'],
    [87.5, '87.5%'],
    [null, '—'],
    [undefined, '—'],
  ] as const) {
    const markup = renderToStaticMarkup(
      <I18nextProvider i18n={i18n}>
        <ChannelCacheRate status={1} rate={rate} />
      </I18nextProvider>
    )
    assert.ok(markup.includes('Last hour'))
    assert.ok(markup.includes(expected))
  }
  for (const status of [2, 3, undefined]) {
    const markup = renderToStaticMarkup(
      <I18nextProvider i18n={i18n}>
        <ChannelCacheRate status={status} rate={87.5} />
      </I18nextProvider>
    )
    assert.equal(markup, '')
  }
})
