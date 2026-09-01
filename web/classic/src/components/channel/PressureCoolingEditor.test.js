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
import { readFileSync } from 'node:fs';
import { describe, test } from 'node:test';

import { mock } from 'bun:test';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

const MockComponent = ({ children }) => createElement('div', null, children);
const MockText = ({ children }) => createElement('span', null, children);

mock.module('@douyinfe/semi-ui', () => ({
  Avatar: MockComponent,
  Card: MockComponent,
  Col: MockComponent,
  InputNumber: MockComponent,
  Radio: MockComponent,
  RadioGroup: MockComponent,
  Row: MockComponent,
  Select: MockComponent,
  Switch: MockComponent,
  Typography: { Text: MockText },
}));
mock.module('@douyinfe/semi-icons', () => ({
  IconBolt: MockComponent,
}));
mock.module('react-i18next', () => ({
  useTranslation: () => ({ t: (key) => key }),
}));

const { default: PressureCoolingEditor } =
  await import('./PressureCoolingEditor.jsx');

import {
  getPressureCoolingGroupOptions,
  getPressureCoolingValidationError,
  isPressureCoolingSaveAllowed,
  normalizePressureCooling,
  parsePressureCooling,
  serializePressureCooling,
} from './PressureCoolingEditor.utils.js';

const editorSource = readFileSync(
  new URL('./PressureCoolingEditor.jsx', import.meta.url),
  'utf8',
);
const modalSource = readFileSync(
  new URL('../table/channels/modals/EditChannelModal.jsx', import.meta.url),
  'utf8',
);
const localeNames = ['en', 'fr', 'ja', 'ru', 'vi', 'zh', 'zh-CN', 'zh-TW'];
const newTranslationKeys = [
  '首字延迟 FRT',
  '始终启用',
  '冷却设置',
  '任一条件满足即冷却',
  '两个条件同时满足才冷却',
  '两个条件共用同一个观察窗口',
];

const renderEditor = (upstreamErrorEnabled) =>
  renderToStaticMarkup(
    createElement(PressureCoolingEditor, {
      value: { enabled: true, upstream_error_enabled: upstreamErrorEnabled },
      onChange: () => {},
    }),
  );

describe('Classic pressure cooling group scope', () => {
  test('treats legacy configuration without scope as channel-wide', () => {
    const normalized = parsePressureCooling(
      JSON.stringify({ enabled: true, frt_threshold_ms: 8000 }),
    );

    assert.equal(normalized.scope, 'channel');
    assert.deepEqual(normalized.cooldown_groups, []);
    assert.equal(isPressureCoolingSaveAllowed(normalized), true);
    assert.deepEqual(JSON.parse(serializePressureCooling(normalized)), {
      enabled: true,
      frt_threshold_ms: 8000,
      trigger_percent: null,
      cooldown_seconds: null,
      observation_window_seconds: null,
    });
  });

  test('uses the current channel groups as scope options', () => {
    assert.deepEqual(getPressureCoolingGroupOptions(['pro', 'cheap', 'pro']), [
      { label: 'pro', value: 'pro' },
      { label: 'cheap', value: 'cheap' },
    ]);
  });

  test('serializes the selected group scope with the backend field names', () => {
    assert.deepEqual(
      JSON.parse(
        serializePressureCooling({
          scope: 'groups',
          cooldown_groups: ['pro'],
        }),
      ),
      {
        enabled: null,
        frt_threshold_ms: null,
        trigger_percent: null,
        cooldown_seconds: null,
        observation_window_seconds: null,
        scope: 'groups',
        cooldown_groups: ['pro'],
      },
    );
  });

  test('cleans cooldown groups that were removed from the channel', () => {
    const normalized = normalizePressureCooling({
      enabled: true,
      scope: 'groups',
      cooldown_groups: ['pro', 'retired'],
    });

    assert.deepEqual(
      JSON.parse(serializePressureCooling(normalized, ['pro', 'cheap']))
        .cooldown_groups,
      ['pro'],
    );
  });

  test('blocks saving a group-scoped configuration with no selected groups', () => {
    assert.equal(
      isPressureCoolingSaveAllowed({
        scope: 'groups',
        cooldown_groups: [],
      }),
      false,
    );
    assert.equal(
      isPressureCoolingSaveAllowed({
        scope: 'channel',
        cooldown_groups: [],
      }),
      true,
    );
  });

  test('uses a channel-wide empty value for missing or invalid JSON', () => {
    assert.equal(parsePressureCooling('').scope, 'channel');
    assert.equal(parsePressureCooling('{').scope, 'channel');
    assert.equal(serializePressureCooling({ enabled: null }), '');
  });

  test('wires the current channel groups into the editor and save guard', () => {
    assert.match(
      modalSource,
      /<PressureCoolingEditor[\s\S]*groups=\{inputs\.groups\}/,
    );
    assert.match(
      modalSource,
      /isPressureCoolingSaveAllowed\(normalizedPCForSave\)/,
    );
    assert.match(
      modalSource,
      /getPressureCoolingValidationError\(\s*inputs\.pressure_cooling\s*,/,
    );
  });

  test('hides group selection outside the group scope', () => {
    assert.match(editorSource, /v\.scope === 'groups' && \(/);
    assert.match(
      editorSource,
      /optionList=\{getPressureCoolingGroupOptions\(groups\)\}/,
    );
  });
});

describe('Classic pressure cooling upstream-error condition', () => {
  test('normalizes the new fields to their legacy-safe defaults', () => {
    const normalized = normalizePressureCooling({
      enabled: true,
      frt_threshold_ms: 8000,
    });

    assert.deepEqual(normalized, {
      enabled: true,
      frt_threshold_ms: 8000,
      trigger_percent: null,
      cooldown_seconds: null,
      observation_window_seconds: null,
      upstream_error_enabled: false,
      upstream_error_trigger_percent: null,
      upstream_error_min_samples: null,
      condition_mode: 'any',
      scope: 'channel',
      cooldown_groups: [],
    });
  });

  test('does not add default upstream-error fields to a legacy payload', () => {
    const serialized = JSON.parse(
      serializePressureCooling(
        normalizePressureCooling({ enabled: true, frt_threshold_ms: 8000 }),
      ),
    );

    assert.deepEqual(serialized, {
      enabled: true,
      frt_threshold_ms: 8000,
      trigger_percent: null,
      cooldown_seconds: null,
      observation_window_seconds: null,
    });
    assert.equal('upstream_error_enabled' in serialized, false);
    assert.equal('condition_mode' in serialized, false);
  });

  test('round trips an enabled upstream-error condition', () => {
    const value = normalizePressureCooling({
      enabled: true,
      upstream_error_enabled: true,
      upstream_error_trigger_percent: 25,
      upstream_error_min_samples: 20,
      condition_mode: 'all',
    });

    assert.deepEqual(JSON.parse(serializePressureCooling(value)), {
      enabled: true,
      frt_threshold_ms: null,
      trigger_percent: null,
      cooldown_seconds: null,
      observation_window_seconds: null,
      upstream_error_enabled: true,
      upstream_error_trigger_percent: 25,
      upstream_error_min_samples: 20,
      condition_mode: 'all',
    });
    assert.deepEqual(
      parsePressureCooling(serializePressureCooling(value)),
      value,
    );
  });

  test('omits upstream-error values equal to global defaults', () => {
    const serialized = JSON.parse(
      serializePressureCooling(
        normalizePressureCooling({
          enabled: true,
          upstream_error_enabled: true,
          upstream_error_trigger_percent: 50,
          upstream_error_min_samples: 10,
          condition_mode: 'any',
        }),
      ),
    );

    assert.equal(serialized.upstream_error_enabled, true);
    assert.equal('upstream_error_trigger_percent' in serialized, false);
    assert.equal('upstream_error_min_samples' in serialized, false);
    assert.equal('condition_mode' in serialized, false);
  });

  test('rejects invalid upstream-error values before saving', () => {
    const invalidValues = [
      { trigger_percent: 0 },
      { trigger_percent: 101 },
      { trigger_percent: 2.5 },
      { upstream_error_trigger_percent: -1 },
      { upstream_error_trigger_percent: 101 },
      { upstream_error_min_samples: 0 },
      { upstream_error_min_samples: 10001 },
      { upstream_error_min_samples: 2.5 },
      { condition_mode: 'sometimes' },
      { upstream_error_enabled: 'true' },
    ];

    for (const value of invalidValues) {
      assert.notEqual(getPressureCoolingValidationError(value), null, value);
      assert.equal(isPressureCoolingSaveAllowed(value), false, value);
    }
    assert.equal(
      getPressureCoolingValidationError({
        trigger_percent: 1,
        upstream_error_trigger_percent: 0,
        upstream_error_min_samples: 1,
        condition_mode: 'any',
        upstream_error_enabled: false,
      }),
      null,
    );
  });

  test('serializes an enabled upstream-error condition with otherwise empty fields', () => {
    const serialized = serializePressureCooling({
      upstream_error_enabled: true,
    });

    assert.notEqual(serialized, '');
    assert.deepEqual(JSON.parse(serialized), {
      enabled: null,
      frt_threshold_ms: null,
      trigger_percent: null,
      cooldown_seconds: null,
      observation_window_seconds: null,
      upstream_error_enabled: true,
    });
  });

  test('wires trigger conditions and collapsible upstream inputs', () => {
    assert.match(editorSource, /t\('触发条件'\)/);
    assert.match(editorSource, /t\('条件组合'\)/);
    assert.match(editorSource, /t\('任一条件满足即冷却'\)/);
    assert.match(editorSource, /t\('两个条件同时满足才冷却'\)/);
    assert.match(editorSource, /t\('上游报错'\)/);
    assert.match(editorSource, /v\.upstream_error_enabled === true && \(/);
    assert.match(editorSource, /t\('最小样本数'\)/);
  });

  test('does not render condition combination when upstream errors are disabled', () => {
    assert.doesNotMatch(renderEditor(false), /条件组合/);
    assert.match(renderEditor(true), /条件组合/);
  });

  test('provides the new pressure-cooling translations in every locale', () => {
    for (const locale of localeNames) {
      const translations = JSON.parse(
        readFileSync(
          new URL(`../../i18n/locales/${locale}.json`, import.meta.url),
          'utf8',
        ),
      ).translation;

      for (const key of newTranslationKeys) {
        assert.equal(typeof translations[key], 'string', `${locale}: ${key}`);
        assert.notEqual(translations[key].trim(), '', `${locale}: ${key}`);
      }
    }
  });
});
