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
import { act, useState } from 'react'
import { I18nextProvider } from 'react-i18next'

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
type DomInputElement = InstanceType<typeof dom.HTMLInputElement>
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
;(
  globalThis as typeof globalThis & {
    IS_REACT_ACT_ENVIRONMENT?: boolean
  }
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

const reactDomClientModule = 'react-dom/client?tag-input-test'
const { createRoot } = await import(reactDomClientModule)
const { TagInput } = await import('./tag-input')

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  interpolation: { escapeValue: false },
})

interface HarnessProps {
  initial?: string[]
  separators?: string[]
  normalize?: (raw: string) => string | null
}

interface HarnessState {
  tags: string[]
}

function Harness(props: HarnessProps & { state: HarnessState }) {
  const [tags, setTags] = useState(props.state.tags)
  props.state.tags = tags

  return (
    <TagInput
      value={tags}
      onChange={setTags}
      separators={props.separators}
      normalize={props.normalize}
    />
  )
}

async function renderHarness(props: HarnessProps = {}) {
  const container = dom.document.createElement('div')
  dom.document.body.append(container)
  const root = createRoot(container)
  const state: HarnessState = { tags: props.initial ?? [] }

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <Harness {...props} state={state} />
      </I18nextProvider>
    )
  })

  const input = container.querySelector('input')
  assert.ok(input, 'TagInput should render an input')

  return {
    container,
    input,
    getTags: () => state.tags,
    async cleanup() {
      await act(async () => root.unmount())
      container.remove()
    },
  }
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

async function pressKey(input: DomInputElement, key: string, keyCode?: number) {
  await act(async () => {
    input.dispatchEvent(
      new dom.KeyboardEvent('keydown', {
        key,
        keyCode,
        which: keyCode,
        bubbles: true,
        cancelable: true,
      })
    )
  })
}

async function dispatchComposition(input: DomInputElement, type: string) {
  await act(async () => {
    input.dispatchEvent(new dom.CompositionEvent(type, { bubbles: true }))
  })
}

describe(
  'TagInput separators and normalization',
  { concurrency: false },
  () => {
    test('commits configured fullwidth comma and space separators', async () => {
      const rendered = await renderHarness({ separators: [',', '，', ' '] })
      try {
        await setInputValue(rendered.input, '200')
        await pressKey(rendered.input, '，')
        await setInputValue(rendered.input, '404')
        await pressKey(rendered.input, ' ')
        await setInputValue(rendered.input, '500, 501， 502')
        await pressKey(rendered.input, 'Enter')

        assert.deepEqual(rendered.getTags(), [
          '200',
          '404',
          '500',
          '501',
          '502',
        ])
      } finally {
        await rendered.cleanup()
      }
    })

    test('applies normalize, discards null, and does not add duplicates', async () => {
      const rendered = await renderHarness({
        normalize: (raw) => {
          const value = raw.trim()
          if (!/^\d+$/.test(value)) return null
          const number = Number(value)
          return number >= 100 && number <= 599 ? String(number) : null
        },
      })
      try {
        await setInputValue(rendered.input, '99')
        await pressKey(rendered.input, 'Enter')
        assert.deepEqual(rendered.getTags(), [])

        await setInputValue(rendered.input, '200')
        await pressKey(rendered.input, 'Enter')
        await setInputValue(rendered.input, '200')
        await pressKey(rendered.input, 'Enter')

        assert.deepEqual(rendered.getTags(), ['200'])
      } finally {
        await rendered.cleanup()
      }
    })
  }
)

describe(
  'TagInput existing keyboard and blur behavior',
  { concurrency: false },
  () => {
    test('keeps the default separator behavior when separators is omitted', async () => {
      const rendered = await renderHarness()
      try {
        await setInputValue(rendered.input, 'alpha')
        await pressKey(rendered.input, ',')
        assert.deepEqual(rendered.getTags(), ['alpha'])

        await setInputValue(rendered.input, 'beta gamma')
        await pressKey(rendered.input, ' ')
        assert.deepEqual(rendered.getTags(), ['alpha'])

        await pressKey(rendered.input, 'Enter')
        assert.deepEqual(rendered.getTags(), ['alpha', 'beta gamma'])
      } finally {
        await rendered.cleanup()
      }
    })

    test('removes the last tag with Backspace and commits on blur', async () => {
      const rendered = await renderHarness({ initial: ['first', 'second'] })
      try {
        await pressKey(rendered.input, 'Backspace')
        assert.deepEqual(rendered.getTags(), ['first'])

        await setInputValue(rendered.input, 'third')
        await act(async () => {
          rendered.input.dispatchEvent(
            new dom.Event('focusout', { bubbles: true })
          )
        })
        assert.deepEqual(rendered.getTags(), ['first', 'third'])

        const removeButtons = rendered.container.querySelectorAll('button')
        assert.equal(removeButtons.length, 2)
        await act(async () => {
          removeButtons[1]?.dispatchEvent(
            new dom.MouseEvent('click', { bubbles: true })
          )
        })
        assert.deepEqual(rendered.getTags(), ['first'])
      } finally {
        await rendered.cleanup()
      }
    })

    test('does not submit Enter while an IME composition is active', async () => {
      const rendered = await renderHarness()
      try {
        await dispatchComposition(rendered.input, 'compositionstart')
        await setInputValue(rendered.input, '中文')
        await pressKey(rendered.input, 'Enter')
        assert.deepEqual(rendered.getTags(), [])

        await dispatchComposition(rendered.input, 'compositionend')
        await pressKey(rendered.input, 'Enter', 229)
        assert.deepEqual(rendered.getTags(), [])

        await pressKey(rendered.input, 'Enter')
        assert.deepEqual(rendered.getTags(), ['中文'])
      } finally {
        await rendered.cleanup()
      }
    })
  }
)
