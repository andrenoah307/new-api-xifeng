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
import { describe, test } from 'node:test'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { renderToStaticMarkup } from 'react-dom/server'

import type { PricingModel } from '../types'
import { buildGeminiSample, ModelDetailsApi } from './model-details-api'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  interpolation: { escapeValue: false },
})

const baseUrl = 'https://gateway.example.test'
const model: PricingModel = {
  id: 1,
  model_name: 'gemini-2.5-flash',
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 1,
  enable_groups: ['default'],
  supported_endpoint_types: ['gemini'],
}
const endpointMap = {
  gemini: {
    path: '/v1beta/models/{model}:generateContent',
    method: 'POST',
  },
}

function renderApi(): string {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  queryClient.setQueryData(['status'], { server_address: baseUrl })

  try {
    return renderToStaticMarkup(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <ModelDetailsApi model={model} endpointMap={endpointMap} />
        </QueryClientProvider>
      </I18nextProvider>
    )
  } finally {
    queryClient.clear()
  }
}

describe('model details API tab', () => {
  test('keeps code samples and authentication without parameter or rate-limit cards', () => {
    const markup = renderApi()

    assert.match(markup, /Code samples/)
    assert.match(markup, /Authentication/)
    assert.match(markup, /All requests must include/)

    assert.doesNotMatch(markup, /Rate\s+limits/)
    assert.doesNotMatch(markup, />RPM</)
    assert.doesNotMatch(markup, />TPM</)
    assert.doesNotMatch(markup, />RPD</)
    assert.doesNotMatch(
      markup,
      /RPM = requests per minute, TPM = tokens per minute, RPD = requests per day\. Limits apply per token group\./
    )
    assert.doesNotMatch(markup, /Supported\s+parameters/)
  })

  test('routes current Gemini SDK samples through the configured gateway', () => {
    const context = {
      baseUrl,
      apiKeyEnv: 'NEW_API_KEY',
      modelName: model.model_name,
      endpointType: 'gemini',
      endpointPath: endpointMap.gemini.path.replace(
        '{model}',
        model.model_name
      ),
    }

    const python = buildGeminiSample('python', context)
    const typescript = buildGeminiSample('typescript', context)

    assert.match(python, /from google import genai/)
    assert.equal(
      python.includes(`http_options={"base_url": "${baseUrl}"}`),
      true
    )
    assert.doesNotMatch(python, /genai\.configure\(api_key=/)

    assert.match(typescript, /from '@google\/genai'/)
    assert.equal(
      typescript.includes(`httpOptions: { baseUrl: '${baseUrl}' }`),
      true
    )
    assert.doesNotMatch(typescript, /new GoogleGenerativeAI\(/)
  })
})
