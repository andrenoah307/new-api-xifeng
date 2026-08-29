/*
Copyright (C) 2025 QuantumNous

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

import assert from 'node:assert/strict';
import { beforeAll, describe, mock, test } from 'bun:test';

import i18next from 'i18next';
import { renderToStaticMarkup } from 'react-dom/server';

function MockComponent({ children }) {
  return children ?? null;
}

mock.module('@douyinfe/semi-ui', () => ({
  Avatar: MockComponent,
  Modal: MockComponent,
  Pagination: MockComponent,
  Tag: MockComponent,
  Toast: {},
  Typography: MockComponent,
}));
mock.module('react-toastify', () => ({
  toast: {},
  ToastContainer: MockComponent,
}));
mock.module('@lobehub/icons', () => {
  const icons = [
    'Ai360',
    'Claude',
    'Cloudflare',
    'Coze',
    'Cohere',
    'DeepSeek',
    'Dify',
    'Doubao',
    'FastGPT',
    'Gemini',
    'Hunyuan',
    'Jina',
    'Jimeng',
    'Kling',
    'LobeHub',
    'Minimax',
    'Mistral',
    'Midjourney',
    'Moonshot',
    'Ollama',
    'OpenAI',
    'OpenRouter',
    'Perplexity',
    'Qwen',
    'Replicate',
    'SiliconCloud',
    'Spark',
    'Suno',
    'Wenxin',
    'XAI',
    'Xinference',
    'Yi',
    'Zhipu',
  ];
  return Object.fromEntries(icons.map((name) => [name, MockComponent]));
});

Object.defineProperty(globalThis, 'window', {
  configurable: true,
  value: {
    matchMedia: () => ({
      matches: false,
      addEventListener: () => {},
      removeEventListener: () => {},
    }),
    addEventListener: () => {},
    removeEventListener: () => {},
  },
});
globalThis.matchMedia = globalThis.window.matchMedia;
Object.defineProperty(globalThis, 'document', {
  configurable: true,
  value: { addEventListener: () => {}, removeEventListener: () => {} },
});

let renderModelPrice;

const storage = new Map();
Object.defineProperty(globalThis, 'localStorage', {
  configurable: true,
  value: {
    getItem: (key) => storage.get(key) ?? null,
    setItem: (key, value) => storage.set(key, String(value)),
    removeItem: (key) => storage.delete(key),
  },
});

beforeAll(async () => {
  ({ renderModelPrice } = await import('./render.jsx'));
  await i18next.init({
    lng: 'en',
    resources: { en: { translation: {} } },
    interpolation: { escapeValue: false },
  });
});

function setDisplayType(type) {
  storage.clear();
  storage.set('quota_display_type', type);
  storage.set('quota_per_unit', '500000');
}

function renderText(opts) {
  const rendered = renderModelPrice(opts);
  return renderToStaticMarkup(rendered).replaceAll(/<[^>]+>/g, '');
}

const responseFixture = {
  prompt_tokens: 100,
  completion_tokens: 7,
  model_ratio: 1,
  completion_ratio: 1,
  group_ratio: 1,
  model_price: -1,
  cache_tokens: 40,
  cache_ratio: 0.1,
  cache_creation_tokens: 10,
  cache_creation_ratio: 1.25,
};

describe('Classic Responses cache billing details', () => {
  test('shows cache read and cache creation in USD display mode', () => {
    setDisplayType('USD');
    const text = renderText(responseFixture);

    assert.match(text, /缓存读取价格：\$0\.200000/);
    assert.match(text, /缓存创建价格：\$2\.500000/);
    assert.match(text, /缓存 40 tokens/);
    assert.match(text, /缓存创建 10 tokens/);
    assert.match(text, /\$0\.000147/);
  });

  test('uses the same weighted cache formula in token display mode', () => {
    setDisplayType('TOKENS');
    const text = renderText(responseFixture);

    assert.match(text, /缓存输入：40 .* = 4/);
    assert.match(text, /缓存创建：10 .* = 12\.5/);
    assert.match(text, /合计：73\.5/);
  });

  test('clamps the ordinary-input remainder when cache exceeds the prompt', () => {
    setDisplayType('TOKENS');
    const text = renderText({
      ...responseFixture,
      prompt_tokens: 10,
      completion_tokens: 0,
      cache_tokens: 20,
      cache_creation_tokens: 5,
    });

    assert.doesNotMatch(text, /普通输入：-[\d.]+/);
    assert.match(text, /合计：8\.5/);
  });

  test('renders split cache-creation buckets with their own ratios', () => {
    setDisplayType('TOKENS');
    const text = renderText({
      ...responseFixture,
      cache_creation_tokens: 20,
      cache_creation_ratio: 1.25,
      cache_creation_tokens_5m: 6,
      cache_creation_ratio_5m: 2,
      cache_creation_tokens_1h: 8,
      cache_creation_ratio_1h: 3,
    });

    assert.match(text, /缓存创建倍率 5m 2 \/ 1h 3/);
    assert.match(text, /5m缓存创建：6 .* = 12/);
    assert.match(text, /1h缓存创建：8 .* = 24/);
  });
});
