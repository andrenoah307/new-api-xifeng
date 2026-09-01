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
*/
import assert from 'node:assert/strict'
import { after, describe, test } from 'node:test'

import { Window } from 'happy-dom'
import { createInstance } from 'i18next'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider } from 'react-i18next'

import { channelFormSchema } from '../../lib/channel-form'

const dom = new Window({ url: 'http://localhost' })
const globals = {
  window: dom,
  document: dom.document,
  navigator: dom.navigator,
  HTMLElement: dom.HTMLElement,
  Element: dom.Element,
  Node: dom.Node,
  Event: dom.Event,
  KeyboardEvent: dom.KeyboardEvent,
  FocusEvent: dom.FocusEvent,
  InputEvent: dom.InputEvent,
  CompositionEvent: dom.CompositionEvent,
  HTMLInputElement: dom.HTMLInputElement,
  HTMLButtonElement: dom.HTMLButtonElement,
  MouseEvent: dom.MouseEvent,
  EventTarget: dom.EventTarget,
  Document: dom.Document,
  DocumentFragment: dom.DocumentFragment,
  Text: dom.Text,
  Comment: dom.Comment,
  DOMRect: dom.DOMRect,
  PointerEvent: dom.PointerEvent,
  matchMedia: dom.matchMedia.bind(dom),
  getComputedStyle: dom.getComputedStyle.bind(dom),
  requestAnimationFrame: dom.requestAnimationFrame.bind(dom),
  cancelAnimationFrame: dom.cancelAnimationFrame.bind(dom),
  localStorage: dom.localStorage,
  customElements: dom.customElements,
  MutationObserver: dom.MutationObserver,
  ResizeObserver: dom.ResizeObserver,
  IntersectionObserver: dom.IntersectionObserver,
  ShadowRoot: dom.ShadowRoot,
}
const globalRecord = globalThis as typeof globalThis & Record<string, unknown>
const previousGlobals = new Map<string, { present: boolean; value: unknown }>()
for (const key of Object.keys(globals)) {
  previousGlobals.set(key, {
    present: Object.hasOwn(globalRecord, key),
    value: globalRecord[key],
  })
}
Object.assign(globalThis, globals)
const previousActEnvironment = {
  present: Object.hasOwn(globalRecord, 'IS_REACT_ACT_ENVIRONMENT'),
  value: globalRecord.IS_REACT_ACT_ENVIRONMENT,
}
;(
  globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true

after(() => {
  for (const [key, previous] of previousGlobals) {
    if (previous.present) {
      globalRecord[key] = previous.value
    } else {
      Reflect.deleteProperty(globalRecord, key)
    }
  }
  if (previousActEnvironment.present) {
    globalRecord.IS_REACT_ACT_ENVIRONMENT = previousActEnvironment.value
  } else {
    Reflect.deleteProperty(globalRecord, 'IS_REACT_ACT_ENVIRONMENT')
  }
})

const {
  normalizePressureCooling,
  parsePressureCooling,
  serializePressureCooling,
  getPressureCoolingGroupOptions,
  isPressureCoolingSaveAllowed,
} = await import('./pressure-cooling')
const { PressureCoolingEditor } = await import('./pressure-cooling-editor')

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  interpolation: { escapeValue: false },
})

describe('pressure cooling scope configuration', () => {
  test('normalizes legacy configuration without scope as channel and avoids scope noise', () => {
    const legacy = JSON.stringify({ enabled: true, frt_threshold_ms: 8000 })
    const normalized = parsePressureCooling(legacy)

    assert.equal(normalized.scope, 'channel')
    assert.equal(normalized.cooldown_groups.length, 0)
    assert.equal(normalized.upstream_error_enabled, false)
    assert.equal(normalized.condition_mode, 'any')
    assert.deepEqual(JSON.parse(serializePressureCooling(normalized)), {
      enabled: true,
      frt_threshold_ms: 8000,
      trigger_percent: null,
      cooldown_seconds: null,
      observation_window_seconds: null,
    })
    assert.equal(
      Object.hasOwn(JSON.parse(serializePressureCooling(normalized)), 'scope'),
      false
    )
  })

  test('uses the channel group field as the specific-scope option source', () => {
    assert.deepEqual(getPressureCoolingGroupOptions(['pro', 'cheap', 'pro']), [
      { value: 'pro', label: 'pro' },
      { value: 'cheap', label: 'cheap' },
    ])

    const watched: string[] = []
    const form = {
      watch: (name: string) => {
        watched.push(name)
        return name === 'group'
          ? ['pro', 'cheap']
          : JSON.stringify({ enabled: true, scope: 'groups' })
      },
      setValue: () => undefined,
    }
    renderToStaticMarkup(
      createElement(
        I18nextProvider,
        { i18n },
        createElement(PressureCoolingEditor, { form: form as never })
      )
    )
    assert.ok(watched.includes('group'))
  })

  test('hides the cooldown group control for the channel scope', () => {
    const form = {
      watch: (name: string) =>
        name === 'group' ? ['pro'] : JSON.stringify({ enabled: true }),
      setValue: () => undefined,
    }
    const markup = renderToStaticMarkup(
      createElement(
        I18nextProvider,
        { i18n },
        createElement(PressureCoolingEditor, { form: form as never })
      )
    )
    assert.match(markup, />Entire Channel</)
    assert.equal(markup.includes('Cooldown Groups'), false)
  })

  test('serializes a selected group with the backend field names', () => {
    const value = normalizePressureCooling({
      enabled: true,
      scope: 'groups',
      cooldown_groups: ['pro'],
    })

    assert.ok(value)
    assert.deepEqual(JSON.parse(serializePressureCooling(value)), {
      enabled: true,
      scope: 'groups',
      cooldown_groups: ['pro'],
      frt_threshold_ms: null,
      trigger_percent: null,
      cooldown_seconds: null,
      observation_window_seconds: null,
    })
  })

  test('round-trips an enabled upstream error condition and omits defaults', () => {
    const value = normalizePressureCooling({
      enabled: true,
      upstream_error_enabled: true,
      upstream_error_trigger_percent: 75,
      upstream_error_min_samples: 25,
      condition_mode: 'all',
    })

    assert.ok(value)
    const serialized = JSON.parse(serializePressureCooling(value))
    assert.deepEqual(serialized, {
      enabled: true,
      frt_threshold_ms: null,
      trigger_percent: null,
      cooldown_seconds: null,
      observation_window_seconds: null,
      upstream_error_enabled: true,
      upstream_error_trigger_percent: 75,
      upstream_error_min_samples: 25,
      condition_mode: 'all',
    })
    assert.deepEqual(parsePressureCooling(JSON.stringify(serialized)), value)

    const withDefaults = normalizePressureCooling({
      enabled: true,
      upstream_error_enabled: true,
      upstream_error_trigger_percent: 50,
      upstream_error_min_samples: 10,
      condition_mode: 'any',
    })
    assert.ok(withDefaults)
    const defaultPayload = JSON.parse(serializePressureCooling(withDefaults))
    assert.equal(defaultPayload.upstream_error_enabled, true)
    assert.equal(Object.hasOwn(defaultPayload, 'upstream_error_trigger_percent'), false)
    assert.equal(Object.hasOwn(defaultPayload, 'upstream_error_min_samples'), false)
    assert.equal(Object.hasOwn(defaultPayload, 'condition_mode'), false)
  })

  test('cleans cooldown groups removed from the channel group field before serialization', () => {
    const value = normalizePressureCooling({
      enabled: true,
      scope: 'groups',
      cooldown_groups: ['pro', 'cheap'],
    })

    assert.ok(value)
    assert.deepEqual(
      JSON.parse(serializePressureCooling(value, ['cheap'])).cooldown_groups,
      ['cheap']
    )
  })

  test('blocks saving a groups-scoped configuration with no selected groups', () => {
    const value = normalizePressureCooling({
      enabled: true,
      scope: 'groups',
      cooldown_groups: [],
    })

    assert.ok(value)
    assert.equal(isPressureCoolingSaveAllowed(value), false)
    assert.equal(
      isPressureCoolingSaveAllowed({ ...value, scope: 'channel' }),
      true
    )

    const result = channelFormSchema.safeParse({
      name: 'channel',
      type: 1,
      key: '',
      models: 'model',
      group: ['pro'],
      status: 1,
      pressure_cooling: JSON.stringify({
        enabled: true,
        scope: 'groups',
        cooldown_groups: [],
      }),
    })
    assert.equal(result.success, false)
    if (!result.success) {
      assert.equal(result.error.issues[0]?.path[0], 'pressure_cooling')
    }
  })

  test('rejects invalid upstream error condition values in the form schema', () => {
    const base = {
      name: 'channel',
      type: 1,
      key: '',
      models: 'model',
      group: ['pro'],
      status: 1,
    }
    const invalidValues = [
      { upstream_error_trigger_percent: -1 },
      { upstream_error_trigger_percent: 101 },
      { upstream_error_min_samples: 0 },
      { upstream_error_min_samples: 10001 },
      { upstream_error_min_samples: 1.5 },
      { condition_mode: 'sometimes' },
    ]

    for (const value of invalidValues) {
      const result = channelFormSchema.safeParse({
        ...base,
        pressure_cooling: JSON.stringify(value),
      })
      assert.equal(result.success, false)
    }
  })

  test('renders the trigger conditions and hides upstream fields when disabled', () => {
    const form = {
      watch: (name: string) =>
        name === 'group' ? ['pro'] : JSON.stringify({ enabled: true }),
      setValue: () => undefined,
    }
    const disabledMarkup = renderToStaticMarkup(
      createElement(
        I18nextProvider,
        { i18n },
        createElement(PressureCoolingEditor, { form: form as never })
      )
    )
    assert.match(disabledMarkup, />Trigger Conditions</)
    assert.match(disabledMarkup, />Upstream Errors</)
    assert.equal(disabledMarkup.includes('Minimum Samples'), false)

    const enabledForm = {
      watch: (name: string) =>
        name === 'group'
          ? ['pro']
          : JSON.stringify({ enabled: true, upstream_error_enabled: true }),
      setValue: () => undefined,
    }
    const enabledMarkup = renderToStaticMarkup(
      createElement(
        I18nextProvider,
        { i18n },
        createElement(PressureCoolingEditor, { form: enabledForm as never })
      )
    )
    assert.match(enabledMarkup, />Error Rate</)
    assert.match(enabledMarkup, />Minimum Samples</)
  })
})
