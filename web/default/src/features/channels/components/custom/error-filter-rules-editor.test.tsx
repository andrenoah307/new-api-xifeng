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
*/
import assert from 'node:assert/strict'
import { after, describe, test } from 'node:test'

import { Window } from 'happy-dom'
import { createInstance } from 'i18next'
import { act, useState } from 'react'
import type { UseFormReturn } from 'react-hook-form'
import { I18nextProvider } from 'react-i18next'

import type { ChannelFormValues } from '../../lib/channel-form'
import type { FilterRule } from '../../lib/error-filter'

const dom = new Window({ url: 'http://localhost' })
const domGlobalKeys = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'Element',
  'Node',
  'Event',
  'KeyboardEvent',
  'FocusEvent',
  'InputEvent',
  'CompositionEvent',
  'HTMLInputElement',
  'HTMLButtonElement',
  'MouseEvent',
  'EventTarget',
  'Document',
  'DocumentFragment',
  'Text',
  'Comment',
  'DOMRect',
  'PointerEvent',
  'matchMedia',
  'getComputedStyle',
  'requestAnimationFrame',
  'cancelAnimationFrame',
  'localStorage',
  'customElements',
  'MutationObserver',
  'ResizeObserver',
  'IntersectionObserver',
  'ShadowRoot',
  'IS_REACT_ACT_ENVIRONMENT',
] as const
const globalRecord = globalThis as typeof globalThis & Record<string, unknown>
const previousGlobals = new Map<string, { present: boolean; value: unknown }>()
for (const key of domGlobalKeys) {
  previousGlobals.set(key, {
    present: Object.hasOwn(globalRecord, key),
    value: globalRecord[key],
  })
}

Object.assign(globalThis, {
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
})
Object.defineProperty(dom.Element.prototype, 'getAnimations', {
  configurable: true,
  value: () => [],
})
;(
  globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true

after(() => {
  for (const key of domGlobalKeys) {
    const previous = previousGlobals.get(key)
    if (!previous) continue
    if (previous.present) {
      ;(globalRecord as Record<string, unknown>)[key] = previous.value
    } else {
      Reflect.deleteProperty(globalRecord, key)
    }
  }
})

const reactDomClientModule = 'react-dom/client?error-filter-rules-editor-test'
const { createRoot } = await import(reactDomClientModule)
const { ErrorFilterRulesEditor } = await import('./error-filter-rules-editor')
const { api } = await import('@/lib/api')

type DomElement = InstanceType<typeof dom.Element>
type DomInputElement = InstanceType<typeof dom.HTMLInputElement>
type DomButtonElement = InstanceType<typeof dom.HTMLButtonElement>

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  interpolation: { escapeValue: false },
})

interface HarnessState {
  raw: string
}

interface EditorHarnessProps {
  initial: string
  channelId?: number
  state: HarnessState
}

function EditorHarness(props: EditorHarnessProps) {
  const [raw, setRaw] = useState(props.initial)
  props.state.raw = raw

  const form = {
    watch: () => raw,
    setValue: (_name: keyof ChannelFormValues, value: unknown) => {
      const next = String(value ?? '')
      props.state.raw = next
      setRaw(next)
    },
  } as unknown as UseFormReturn<ChannelFormValues>

  return <ErrorFilterRulesEditor form={form} channelId={props.channelId} />
}

async function renderEditor(initial: string, channelId?: number) {
  const container = dom.document.createElement('div')
  dom.document.body.append(container)
  const root = createRoot(container)
  const state: HarnessState = { raw: initial }

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <EditorHarness initial={initial} channelId={channelId} state={state} />
      </I18nextProvider>
    )
  })

  return {
    container,
    getRaw: () => state.raw,
    async cleanup() {
      await act(async () => root.unmount())
      container.remove()
      for (const child of dom.document.body.children) {
        if (child !== container) child.remove()
      }
    },
  }
}

function findButton(container: DomElement, text: string): DomButtonElement {
  let button: DomButtonElement | undefined
  for (const candidate of container.querySelectorAll('button')) {
    if (candidate.textContent?.includes(text)) {
      button = candidate as DomButtonElement
      break
    }
  }
  assert.ok(button, `button containing ${text} should exist`)
  return button
}

function findTagInputs(container: DomElement): DomInputElement[] {
  const inputs: DomInputElement[] = []
  for (const input of container.querySelectorAll('input')) {
    if (input.type === 'text') inputs.push(input as DomInputElement)
  }
  return inputs
}

async function setInputValue(input: DomInputElement, value: string) {
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(
      dom.HTMLInputElement.prototype,
      'value'
    )?.set
    assert.ok(setter, 'happy-dom should expose the input value setter')
    setter.call(input, value)
    input.dispatchEvent(new dom.Event('input', { bubbles: true }))
  })
}

async function pressKey(input: DomInputElement, key: string) {
  await act(async () => {
    input.dispatchEvent(
      new dom.KeyboardEvent('keydown', {
        key,
        bubbles: true,
        cancelable: true,
      })
    )
  })
}

async function flushEffects() {
  await act(async () => {
    await Promise.resolve()
    await Promise.resolve()
  })
}

describe('error filter editor rule behavior', { concurrency: false }, () => {
  test('preserves every message keyword while editing other tag fields', async () => {
    const initial = JSON.stringify([
      {
        action: 'retry',
        status_codes: [],
        message_contains: ['first keyword', 'second keyword'],
        error_codes: [],
      },
    ])
    const rendered = await renderEditor(initial)

    try {
      const inputs = findTagInputs(rendered.container)
      assert.ok(inputs.length >= 3)
      const statusInput = inputs[0]
      assert.ok(statusInput)
      await setInputValue(statusInput, '99, 429, 600')
      await pressKey(statusInput, 'Enter')

      assert.deepEqual(JSON.parse(rendered.getRaw()), [
        {
          status_codes: [429],
          message_contains: ['first keyword', 'second keyword'],
          error_codes: [],
          action: 'retry',
          rewrite_message: '',
          replace_status_code: 200,
          replace_message: '',
        },
      ])
    } finally {
      await rendered.cleanup()
    }
  })

  test('uses tolerant parsing to fill missing fields before serializing updates', async () => {
    const rendered = await renderEditor(
      JSON.stringify([{ action: 'rewrite', message_contains: ['a', 'b'] }])
    )

    try {
      const inputs = findTagInputs(rendered.container)
      const errorInput = inputs[1]
      assert.ok(errorInput)
      await setInputValue(errorInput, 'upstream_error')
      await pressKey(errorInput, 'Enter')

      const [rule] = JSON.parse(rendered.getRaw()) as FilterRule[]
      assert.deepEqual(rule?.message_contains, ['a', 'b'])
      assert.deepEqual(rule?.error_codes, ['upstream_error'])
      assert.equal(rule?.action, 'rewrite')
      assert.equal(rule?.replace_status_code, 200)
    } finally {
      await rendered.cleanup()
    }
  })

  test('adds a message keyword through the multi-value TagInput', async () => {
    const rendered = await renderEditor(
      JSON.stringify([{ action: 'retry', message_contains: ['first'] }])
    )
    try {
      const inputs = findTagInputs(rendered.container)
      const messageInput = inputs[2]
      assert.ok(messageInput)
      await setInputValue(messageInput, 'second')
      await pressKey(messageInput, 'Enter')
      const [rule] = JSON.parse(rendered.getRaw()) as FilterRule[]
      assert.deepEqual(rule?.message_contains, ['first', 'second'])
    } finally {
      await rendered.cleanup()
    }
  })

  test('does not expose the recent-record picker without a channel id', async () => {
    const rendered = await renderEditor('[]')
    try {
      await act(async () => {
        findButton(rendered.container, 'Add Rule').click()
      })
      await flushEffects()
      assert.equal(
        (() => {
          for (const button of rendered.container.querySelectorAll('button')) {
            if (button.textContent?.includes('Select from error records')) {
              return true
            }
          }
          return false
        })(),
        false
      )
    } finally {
      await rendered.cleanup()
    }
  })

  test('removes a rule without changing the remaining rule order', async () => {
    const rendered = await renderEditor(
      JSON.stringify([
        { action: 'retry', status_codes: [500] },
        { action: 'rewrite', message_contains: ['second'] },
      ])
    )
    try {
      const deleteButton = rendered.container.querySelector(
        '[title="Delete rule"]'
      ) as DomButtonElement | null
      assert.ok(deleteButton)
      await act(async () => {
        deleteButton.click()
      })
      const rules = JSON.parse(rendered.getRaw()) as FilterRule[]
      assert.equal(rules.length, 1)
      assert.equal(rules[0]?.action, 'rewrite')
    } finally {
      await rendered.cleanup()
    }
  })

  test('keeps rewrite and replace action parameters in the form JSON', async () => {
    const rewrite = await renderEditor(
      JSON.stringify([{ action: 'rewrite', message_contains: ['upstream'] }])
    )
    try {
      const rewriteInput = rewrite.container.querySelector(
        '#error-filter-rewrite-0'
      ) as DomInputElement | null
      assert.ok(rewriteInput)
      await setInputValue(rewriteInput, 'rewritten')
      const [rewriteRule] = JSON.parse(rewrite.getRaw()) as FilterRule[]
      assert.equal(rewriteRule?.rewrite_message, 'rewritten')
    } finally {
      await rewrite.cleanup()
    }

    const replace = await renderEditor(
      JSON.stringify([{ action: 'replace', status_codes: [500] }])
    )
    try {
      const statusInput = replace.container.querySelector(
        '#error-filter-replace-status-0'
      ) as DomInputElement | null
      const messageInput = replace.container.querySelector(
        '#error-filter-replace-message-0'
      ) as DomInputElement | null
      assert.ok(statusInput)
      assert.ok(messageInput)
      await setInputValue(statusInput, '201')
      const validRule = JSON.parse(replace.getRaw()) as FilterRule[]
      assert.equal(validRule[0]?.replace_status_code, 201)
      await setInputValue(statusInput, '700')
      await setInputValue(messageInput, 'replacement')
      const [replaceRule] = JSON.parse(replace.getRaw()) as FilterRule[]
      assert.equal(replaceRule?.replace_status_code, 200)
      assert.equal(replaceRule?.replace_message, 'replacement')
    } finally {
      await replace.cleanup()
    }
  })
})

describe('recent error record picker', { concurrency: false }, () => {
  test('requests exactly fifty records, hides metadata, deduplicates, and applies selected values', async () => {
    const originalAdapter = api.defaults.adapter
    let requestUrl = ''
    api.defaults.adapter = async (config) => {
      requestUrl = config.url ?? ''
      return {
        data: {
          success: true,
          data: {
            total: 999999,
            page: 1,
            page_size: 50,
            items: [
              {
                id: 10,
                created_at: 1710000000,
                content:
                  'status_code=503, upstream unavailable (request id: secret)',
                channel_name: 'must not render',
                model_name: 'must not render',
                other: JSON.stringify({
                  status_code: 503,
                  error_code: 'upstream_error',
                  admin_info: { caller_ip: 'must not render' },
                }),
              },
              {
                id: 11,
                created_at: 1710000001,
                content:
                  'status_code=503, upstream unavailable (request id: duplicate)',
                other: JSON.stringify({
                  status_code: 503,
                  error_code: 'upstream_error',
                }),
              },
            ],
          },
        },
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      }
    }

    const rendered = await renderEditor(
      JSON.stringify([
        {
          status_codes: [],
          message_contains: ['existing'],
          error_codes: [],
          action: 'retry',
        },
      ]),
      306
    )

    try {
      await act(async () => {
        findButton(rendered.container, 'Select from error records').click()
      })
      await flushEffects()
      const modalRoot = dom.document.body

      assert.match(requestUrl, /type=5/)
      assert.match(requestUrl, /channel=306/)
      assert.match(requestUrl, /p=1/)
      assert.match(requestUrl, /page_size=50/)
      assert.match(requestUrl, /total_count=50/)
      assert.match(modalRoot.textContent ?? '', /upstream unavailable/)
      assert.match(modalRoot.textContent ?? '', /503/)
      assert.match(modalRoot.textContent ?? '', /upstream_error/)
      assert.doesNotMatch(modalRoot.textContent ?? '', /must not render/)
      assert.doesNotMatch(modalRoot.textContent ?? '', /999999/)
      assert.equal(
        (modalRoot.textContent ?? '').match(/upstream unavailable/g)?.length,
        1
      )

      const checkboxes = modalRoot.querySelectorAll('[data-slot="checkbox"]')
      assert.equal(checkboxes.length, 1)
      await act(async () => {
        const record = modalRoot.querySelector('[data-error-record="true"]')
        assert.ok(record, 'the deduplicated error record should render')
        record.dispatchEvent(new dom.MouseEvent('click', { bubbles: true }))
      })
      await act(async () => {
        findButton(modalRoot, 'Apply selected').click()
      })
      await flushEffects()

      const [rule] = JSON.parse(rendered.getRaw()) as FilterRule[]
      assert.deepEqual(rule?.status_codes, [503])
      assert.deepEqual(rule?.error_codes, ['upstream_error'])
      assert.deepEqual(rule?.message_contains, [
        'existing',
        'upstream unavailable',
      ])
    } finally {
      api.defaults.adapter = originalAdapter
      await rendered.cleanup()
    }
  })

  test('uses generic local copy when the recent-record request fails', async () => {
    const originalAdapter = api.defaults.adapter
    api.defaults.adapter = async (config) => ({
      data: {
        success: false,
        message: 'sensitive backend details must stay hidden',
      },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    })
    const rendered = await renderEditor(
      JSON.stringify([{ action: 'retry', status_codes: [500] }]),
      306
    )

    try {
      await act(async () => {
        findButton(rendered.container, 'Select from error records').click()
      })
      await flushEffects()
      const text = dom.document.body.textContent ?? ''
      assert.match(text, /Failed to load recent error records/)
      assert.doesNotMatch(text, /sensitive backend details/)
    } finally {
      api.defaults.adapter = originalAdapter
      await rendered.cleanup()
    }
  })

  test('renders safe placeholders for incomplete records and supports keyboard selection', async () => {
    const originalAdapter = api.defaults.adapter
    api.defaults.adapter = async (config) => ({
      data: {
        success: true,
        data: {
          items: [{ content: '', other: '{bad json', created_at: 0 }],
          total: 1,
          page: 1,
          page_size: 50,
        },
      },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    })
    const rendered = await renderEditor('[]', 306)

    try {
      await act(async () => {
        findButton(rendered.container, 'Add Rule').click()
      })
      await flushEffects()
      await act(async () => {
        findButton(rendered.container, 'Select from error records').click()
      })
      await flushEffects()
      const record = dom.document.body.querySelector(
        '[data-error-record="true"]'
      )
      assert.ok(record)
      assert.match(dom.document.body.textContent ?? '', /No message content/)
      assert.match(dom.document.body.textContent ?? '', /-/)
      await act(async () => {
        record.dispatchEvent(
          new dom.KeyboardEvent('keydown', {
            key: ' ',
            bubbles: true,
            cancelable: true,
          })
        )
        record.dispatchEvent(
          new dom.KeyboardEvent('keydown', {
            key: 'Enter',
            bubbles: true,
            cancelable: true,
          })
        )
      })
      assert.equal(record.getAttribute('aria-pressed'), 'false')
      await act(async () => {
        findButton(dom.document.body, 'Cancel').click()
      })
    } finally {
      api.defaults.adapter = originalAdapter
      await rendered.cleanup()
    }
  })
})

describe('collapsed rule summary', () => {
  test('uses action-specific summary text and a bounded condition preview', async () => {
    const rendered = await renderEditor(
      JSON.stringify([
        {
          status_codes: [429, 500],
          message_contains: ['one', 'two', 'three'],
          error_codes: ['a', 'b', 'c'],
          action: 'replace',
        },
      ])
    )
    try {
      await act(async () => {
        const collapseButton = rendered.container.querySelector(
          '[title="Collapse rule"]'
        )
        assert.ok(collapseButton)
        collapseButton.dispatchEvent(
          new dom.MouseEvent('click', { bubbles: true })
        )
      })
      const text = rendered.container.textContent ?? ''
      assert.match(text, /Replace/)
      assert.match(text, /429 \/ 500/)
      assert.match(text, /"one", "two"…/)
      assert.match(text, /a, b…/)
    } finally {
      await rendered.cleanup()
    }
  })

  test('keeps the backend-safe warning in an empty collapsed rule summary', async () => {
    const rendered = await renderEditor(JSON.stringify([{ action: 'retry' }]))
    try {
      await act(async () => {
        const collapseButton = rendered.container.querySelector(
          '[title="Collapse rule"]'
        )
        assert.ok(collapseButton)
        collapseButton.dispatchEvent(
          new dom.MouseEvent('click', { bubbles: true })
        )
      })
      assert.match(
        rendered.container.textContent ?? '',
        /No conditions set — this rule will not match any errors/
      )
    } finally {
      await rendered.cleanup()
    }
  })
})
