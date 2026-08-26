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
import { act, type ReactNode, useState } from 'react'
import { I18nextProvider } from 'react-i18next'

import { handleChannelFormKeyDown } from '../../lib/channel-form-keyboard'

const dom = new Window({ url: 'http://localhost' })
const domGlobalKeys = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'Element',
  'Node',
  'NodeFilter',
  'Event',
  'KeyboardEvent',
  'FocusEvent',
  'InputEvent',
  'CompositionEvent',
  'HTMLInputElement',
  'HTMLTextAreaElement',
  'HTMLButtonElement',
  'HTMLAnchorElement',
  'HTMLFormElement',
  'HTMLDivElement',
  'HTMLSelectElement',
  'HTMLOptionElement',
  'SVGElement',
  'MouseEvent',
  'PointerEvent',
  'EventTarget',
  'Document',
  'DocumentFragment',
  'Text',
  'Comment',
  'DOMRect',
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
  NodeFilter: dom.NodeFilter,
  Event: dom.Event,
  KeyboardEvent: dom.KeyboardEvent,
  FocusEvent: dom.FocusEvent,
  InputEvent: dom.InputEvent,
  CompositionEvent: dom.CompositionEvent,
  HTMLInputElement: dom.HTMLInputElement,
  HTMLTextAreaElement: dom.HTMLTextAreaElement,
  HTMLButtonElement: dom.HTMLButtonElement,
  HTMLAnchorElement: dom.HTMLAnchorElement,
  HTMLFormElement: dom.HTMLFormElement,
  HTMLDivElement: dom.HTMLDivElement,
  HTMLSelectElement: dom.HTMLSelectElement,
  HTMLOptionElement: dom.HTMLOptionElement,
  SVGElement: dom.SVGElement,
  MouseEvent: dom.MouseEvent,
  PointerEvent: dom.PointerEvent,
  EventTarget: dom.EventTarget,
  Document: dom.Document,
  DocumentFragment: dom.DocumentFragment,
  Text: dom.Text,
  Comment: dom.Comment,
  DOMRect: dom.DOMRect,
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

const reactDomClientModule = 'react-dom/client?channel-mutate-drawer-test'
const { createRoot } = await import(reactDomClientModule)
const { MultiSelect } = await import('@/components/multi-select')
const { Combobox } = await import('@/components/ui/combobox')
const { Button } = await import('@/components/ui/button')
const { ChannelCustomSections } =
  await import('../custom/channel-custom-sections')

type DomElement = InstanceType<typeof dom.Element>
type DomHTMLElement = InstanceType<typeof dom.HTMLElement>
type DomFormElement = InstanceType<typeof dom.HTMLFormElement>
type DomInputElement = InstanceType<typeof dom.HTMLInputElement>

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  interpolation: { escapeValue: false },
})

function keydown(
  target: DomElement,
  init: { key?: string; keyCode?: number; isComposing?: boolean } = {}
) {
  const event = new dom.KeyboardEvent('keydown', {
    key: init.key ?? 'Enter',
    keyCode: init.keyCode,
    which: init.keyCode,
    isComposing: init.isComposing,
    bubbles: true,
    cancelable: true,
  })
  target.dispatchEvent(event)
  return event
}

function GuardedForm(props: { children: ReactNode; onSubmit: () => void }) {
  return (
    <form
      id='channel-form'
      onKeyDown={handleChannelFormKeyDown}
      onSubmit={(event) => {
        event.preventDefault()
        props.onSubmit()
      }}
    >
      {props.children}
    </form>
  )
}

async function renderHarness(
  children: ReactNode,
  onSubmit: () => void
): Promise<{
  container: DomHTMLElement
  form: DomFormElement
  cleanup: () => Promise<void>
}> {
  const container = dom.document.createElement('div')
  dom.document.body.append(container)
  const root = createRoot(container)
  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <GuardedForm onSubmit={onSubmit}>{children}</GuardedForm>
      </I18nextProvider>
    )
  })
  const form = container.querySelector('form')
  assert.ok(form)
  return {
    container,
    form,
    async cleanup() {
      await act(async () => root.unmount())
      container.remove()
    },
  }
}

describe('channel form Enter guard', { concurrency: false }, () => {
  test('does not submit for a regular input Enter', async () => {
    let submitCount = 0
    const rendered = await renderHarness(<input />, () => submitCount++)
    try {
      const input = rendered.container.querySelector('input')
      assert.ok(input)
      const event = keydown(input)
      if (!event.defaultPrevented) {
        rendered.form.dispatchEvent(
          new dom.Event('submit', { bubbles: true, cancelable: true })
        )
      }
      assert.equal(event.defaultPrevented, true)
      assert.equal(submitCount, 0)
    } finally {
      await rendered.cleanup()
    }
  })

  test('preserves textarea Enter for line breaks', async () => {
    const rendered = await renderHarness(
      <textarea
        onKeyDown={(event) => {
          if (!event.defaultPrevented) {
            event.currentTarget.value += '\n'
          }
        }}
      />,
      () => {
        throw new Error('textarea Enter must not submit')
      }
    )
    try {
      const textarea = rendered.container.querySelector('textarea')
      assert.ok(textarea)
      const event = keydown(textarea)
      assert.equal(event.defaultPrevented, false)
      assert.equal(textarea.value, '\n')
    } finally {
      await rendered.cleanup()
    }
  })

  test('preserves Enter activation for buttons and links', async () => {
    let buttonActivations = 0
    let linkActivations = 0
    const rendered = await renderHarness(
      <>
        <button type='button' onClick={() => buttonActivations++}>
          <span>Run</span>
        </button>
        <a href='/target' onClick={() => linkActivations++}>
          <span>Open</span>
        </a>
      </>,
      () => {
        throw new Error('semantic controls must not submit')
      }
    )
    try {
      const button = rendered.container.querySelector('button span')
      const link = rendered.container.querySelector('a span')
      assert.ok(button)
      assert.ok(link)
      assert.equal(keydown(button).defaultPrevented, false)
      assert.equal(keydown(link).defaultPrevented, false)
      button.dispatchEvent(new dom.MouseEvent('click', { bubbles: true }))
      link.dispatchEvent(new dom.MouseEvent('click', { bubbles: true }))
      assert.equal(buttonActivations, 1)
      assert.equal(linkActivations, 1)
    } finally {
      await rendered.cleanup()
    }
  })

  test('preserves contenteditable Enter and blocks non-href anchors', async () => {
    const rendered = await renderHarness(
      <>
        <div contentEditable='true' />
        <a href='/target'>a link</a>
        <a>not a link</a>
      </>,
      () => {
        throw new Error('semantic controls must not submit')
      }
    )
    try {
      const editable = rendered.container.querySelector('[contenteditable]')
      const hrefAnchor = rendered.container.querySelector('a[href]')
      const anchor = rendered.container.querySelector('a:not([href])')
      assert.ok(editable)
      assert.ok(hrefAnchor)
      assert.ok(anchor)
      assert.equal(keydown(editable).defaultPrevented, false)
      assert.equal(keydown(hrefAnchor).defaultPrevented, false)
      assert.equal(keydown(anchor).defaultPrevented, true)
    } finally {
      await rendered.cleanup()
    }
  })

  test('recognizes semantic targets when DOM helpers are unavailable', () => {
    const shouldPrevent = (target: unknown) => {
      let prevented = false
      handleChannelFormKeyDown({
        defaultPrevented: false,
        key: 'Enter',
        keyCode: 0,
        nativeEvent: { isComposing: false, keyCode: 0 },
        target,
        preventDefault: () => {
          prevented = true
        },
      } as unknown as Parameters<typeof handleChannelFormKeyDown>[0])
      return prevented
    }

    assert.equal(shouldPrevent({ tagName: 'TEXTAREA' }), false)
    assert.equal(shouldPrevent({ tagName: 'BUTTON' }), false)
    assert.equal(shouldPrevent({ tagName: 'A', href: '/target' }), false)
    assert.equal(shouldPrevent({ isContentEditable: true }), false)
    assert.equal(shouldPrevent({ tagName: 'A' }), true)
  })

  test('leaves already-prevented child events untouched', async () => {
    let childCalled = false
    let childSawPrevented = false
    const rendered = await renderHarness(
      <input
        onKeyDown={(event) => {
          childCalled = true
          childSawPrevented = event.defaultPrevented
          event.preventDefault()
        }}
      />,
      () => {
        throw new Error('child-handled Enter must not submit')
      }
    )
    try {
      const input = rendered.container.querySelector('input')
      assert.ok(input)
      const event = keydown(input)
      assert.equal(childCalled, true)
      assert.equal(childSawPrevented, false)
      assert.equal(event.defaultPrevented, true)
    } finally {
      await rendered.cleanup()
    }
  })

  test('does not prevent Enter during IME composition', async () => {
    const rendered = await renderHarness(<input />, () => {
      throw new Error('IME Enter must not submit')
    })
    try {
      const input = rendered.container.querySelector('input')
      assert.ok(input)
      assert.equal(
        keydown(input, { isComposing: true }).defaultPrevented,
        false
      )
      assert.equal(keydown(input, { keyCode: 229 }).defaultPrevented, false)
    } finally {
      await rendered.cleanup()
    }
  })

  test('keeps MultiSelect highlighted selection and custom creation working', async () => {
    const state: { selected: string[] } = { selected: [] }
    function MultiSelectHarness() {
      const [selected, setSelected] = useState<string[]>([])
      state.selected = selected
      return (
        <MultiSelect
          options={[{ value: 'gpt-4o', label: 'GPT-4o' }]}
          selected={selected}
          onChange={setSelected}
          allowCreate
          placeholder='Select models'
        />
      )
    }
    const rendered = await renderHarness(<MultiSelectHarness />, () => {
      throw new Error('MultiSelect Enter must not submit')
    })
    try {
      const input = rendered.container.querySelector(
        '[data-slot="combobox-chip-input"]'
      ) as DomInputElement | null
      assert.ok(input)
      await act(async () => {
        input.focus()
      })
      await act(async () => {
        input.dispatchEvent(
          new dom.KeyboardEvent('keydown', {
            key: 'ArrowDown',
            bubbles: true,
            cancelable: true,
          })
        )
      })
      await act(async () => {})
      const highlighted = dom.document.querySelector(
        '[data-slot="combobox-content"] [data-highlighted]'
      )
      assert.ok(highlighted)
      await act(async () => {
        keydown(input)
      })
      assert.deepEqual(state.selected, ['gpt-4o'])

      await act(async () => {
        const setter = Object.getOwnPropertyDescriptor(
          dom.HTMLInputElement.prototype,
          'value'
        )?.set
        assert.ok(setter)
        setter.call(input, 'custom-model')
        input.dispatchEvent(new dom.Event('input', { bubbles: true }))
      })
      await act(async () => {
        keydown(input)
      })
      assert.deepEqual(state.selected, ['gpt-4o', 'custom-model'])
    } finally {
      await rendered.cleanup()
    }
  })

  test('keeps Legacy Combobox selection working', async () => {
    let selected = ''
    const rendered = await renderHarness(
      <Combobox
        options={[
          { value: 'openai', label: 'OpenAI' },
          { value: 'anthropic', label: 'Anthropic' },
        ]}
        value={selected}
        onValueChange={(value) => {
          selected = value ?? ''
        }}
        openOnFocus
      />,
      () => {
        throw new Error('Legacy Combobox Enter must not submit')
      }
    )
    try {
      const input = rendered.container.querySelector(
        'input[role="combobox"]'
      ) as DomInputElement | null
      assert.ok(input)
      await act(async () => {
        input.focus()
      })
      await act(async () => {
        input.dispatchEvent(
          new dom.KeyboardEvent('keydown', {
            key: 'ArrowDown',
            bubbles: true,
            cancelable: true,
          })
        )
      })
      await act(async () => {
        keydown(input)
      })
      assert.equal(selected, 'openai')
    } finally {
      await rendered.cleanup()
    }
  })

  test('does not submit from any of the four custom extension cards', async () => {
    let submitCount = 0
    const values: Record<string, string> = {
      pressure_cooling: '',
      channel_rate_limit: '',
      error_filter_rules: JSON.stringify([
        {
          status_codes: [429],
          message_contains: ['busy'],
          error_codes: ['rate_limit'],
          action: 'retry',
          rewrite_message: '',
          replace_status_code: 200,
          replace_message: '',
        },
      ]),
      risk_control_headers: JSON.stringify([
        { name: 'X-User', source: 'custom', value: '{user_id}' },
      ]),
    }
    const form = {
      watch: (name: string) => values[name] ?? '',
      setValue: (name: string, value: string) => {
        values[name] = value
      },
    }
    const rendered = await renderHarness(
      <ChannelCustomSections
        form={form as never}
        channelId={1}
        sectionIds={{
          pressureCooling: 'pressure',
          rateLimit: 'rate-limit',
          errorFilter: 'error-filter',
          riskHeaders: 'risk-headers',
        }}
        configured={{
          pressureCooling: false,
          rateLimit: false,
          errorFilter: true,
          riskHeaders: true,
        }}
      />,
      () => submitCount++
    )
    try {
      const sectionIds = [
        'pressure',
        'rate-limit',
        'error-filter',
        'risk-headers',
      ]
      for (const sectionId of sectionIds) {
        const section = rendered.container.querySelector(`#${sectionId}`)
        assert.ok(section)
        const input = section.querySelector('input')
        assert.ok(input, `${sectionId} should contain an input`)
        const event = keydown(input)
        if (!event.defaultPrevented) {
          rendered.form.dispatchEvent(
            new dom.Event('submit', { bubbles: true, cancelable: true })
          )
        }
        assert.equal(event.defaultPrevented, true)
      }
      assert.equal(submitCount, 0)
    } finally {
      await rendered.cleanup()
    }
  })

  test('allows the associated save button to submit the form', async () => {
    let submitCount = 0
    const container = dom.document.createElement('div')
    dom.document.body.append(container)
    const root = createRoot(container)
    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <form
            id='channel-form'
            onKeyDown={handleChannelFormKeyDown}
            onSubmit={(event) => {
              event.preventDefault()
              submitCount++
            }}
          >
            <input />
          </form>
          <Button form='channel-form' type='submit'>
            Save
          </Button>
        </I18nextProvider>
      )
    })
    try {
      const button = container.querySelector('button')
      assert.ok(button)
      await act(async () => {
        button.dispatchEvent(new dom.MouseEvent('click', { bubbles: true }))
      })
      assert.equal(submitCount, 1)
    } finally {
      await act(async () => root.unmount())
      container.remove()
    }
  })
})
