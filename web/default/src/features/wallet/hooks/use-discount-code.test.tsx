import assert from 'node:assert/strict'
import { test } from 'node:test'

import { Window } from 'happy-dom'
import { createInstance } from 'i18next'
import { act } from 'react'
import { I18nextProvider } from 'react-i18next'

test('clearing an applied code unlocks the input and removes the code used for payment', async () => {
  const dom = new Window({ url: 'http://localhost' })
  const globals = {
    window: dom,
    document: dom.document,
    navigator: dom.navigator,
    HTMLElement: dom.HTMLElement,
    Element: dom.Element,
    Node: dom.Node,
    IS_REACT_ACT_ENVIRONMENT: true,
  }
  const previous = Object.getOwnPropertyDescriptors(globalThis)
  Object.assign(globalThis, globals)
  const { createRoot } = await import('react-dom/client')
  const { api } = await import('@/lib/api')
  const { useDiscountCode } = await import('./use-discount-code')
  const { RechargeFormCard } = await import('../components/recharge-form-card')
  const adapter = api.defaults.adapter
  api.defaults.adapter = async (config) => ({
    data: {
      success: true,
      data: { code: 'SAVE', discount_rate: 90, max_amount: 100 },
    },
    status: 200,
    statusText: 'OK',
    headers: {},
    config,
  })
  const i18n = createInstance()
  await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })
  let discount: ReturnType<typeof useDiscountCode> | undefined
  function Harness() {
    discount = useDiscountCode()
    return (
      <I18nextProvider i18n={i18n}>
        <RechargeFormCard
          topupInfo={{
            enable_online_topup: true,
            enable_stripe_topup: false,
            pay_methods: [{ name: 'Alipay', type: 'alipay' }],
            min_topup: 1,
            stripe_min_topup: 1,
            amount_options: [],
            discount: {},
          }}
          presetAmounts={[]}
          selectedPreset={null}
          onSelectPreset={() => {}}
          topupAmount={101}
          onTopupAmountChange={() => {}}
          paymentAmount={0}
          calculating={false}
          onPaymentMethodSelect={() => {}}
          paymentLoading={null}
          redemptionCode=''
          onRedemptionCodeChange={() => {}}
          onRedeem={() => {}}
          redeeming={false}
          discountCode={discount.discountCode}
          discountInfo={discount.discountInfo}
          onDiscountCodeChange={discount.setDiscountCode}
          onValidateDiscountCode={discount.validateDiscountCode}
          onClearDiscountCode={discount.clearDiscountCode}
        />
      </I18nextProvider>
    )
  }
  const container = dom.document.createElement('div')
  dom.document.body.append(container)
  const root = createRoot(container as unknown as HTMLElement)
  try {
    await act(async () => root.render(<Harness />))
    await act(async () => discount?.setDiscountCode('SAVE'))
    await act(async () => {
      await discount?.validateDiscountCode()
    })
    assert.equal(discount?.discountInfo?.max_amount, 100)
    assert.equal(
      container
        .querySelector('input#topup-amount')
        ?.getAttribute('aria-invalid'),
      'true'
    )
    assert.match(
      container.querySelector('#discount-limit-error')?.textContent ?? '',
      /100/
    )
    const paymentButton = [...container.querySelectorAll('button')].find(
      (button) => button.textContent === 'Alipay'
    )
    assert.ok(paymentButton)
    assert.equal(paymentButton.disabled, true)
    assert.equal(
      container.querySelector('input#discount-code')?.hasAttribute('disabled'),
      true
    )
    const clear = [...container.querySelectorAll('button')].find(
      (button) => button.textContent === 'Clear'
    )
    assert.ok(clear)
    await act(async () => clear.click())
    assert.equal(discount?.discountInfo, null)
    assert.equal(discount?.discountCode, '')
    assert.equal(paymentButton.disabled, false)
    assert.equal(container.querySelector('#discount-limit-error'), null)
    assert.equal(
      container.querySelector('input#discount-code')?.hasAttribute('disabled'),
      false
    )
  } finally {
    await act(async () => root.unmount())
    api.defaults.adapter = adapter
    for (const key of Object.keys(globals)) {
      if (previous[key]) Object.defineProperty(globalThis, key, previous[key])
      else Reflect.deleteProperty(globalThis, key)
    }
    await dom.happyDOM.close()
  }
})
