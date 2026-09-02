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
  '压力冷却',
  'FRT 阈值 (ms)，留空默认 8000',
  '触发百分比 (%)，留空默认 50',
  '观察窗口 (秒)，留空默认 60',
  '冷却时长 (秒)，留空默认 300',
  '当渠道首字延迟持续过高时，自动禁用渠道并在冷却后恢复；未开启则使用默认配置',
  '首字延迟 FRT',
  '始终启用',
  '冷却设置',
  '任一条件满足即冷却',
  '两个条件同时满足才冷却',
  '两个条件共用同一个观察窗口',
  '条件满足其一即触发',
  '填 0 表示不启用该条件',
  '最小样本数是比例条件的最小尝试数（分母）',
  '错误数阈值需结合观察窗口理解，高流量渠道慎用',
  '错误数阈值',
  '冷却剩余 {{seconds}} 秒',
];

const renderEditor = (value) =>
  renderToStaticMarkup(
    createElement(PressureCoolingEditor, {
      value: { enabled: true, ...value },
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
      upstream_error_trigger_percent: null,
      upstream_error_min_samples: null,
      upstream_error_trigger_count: null,
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

  test('ignores the removed upstream-error switch while preserving other fields', () => {
    const normalized = parsePressureCooling(
      JSON.stringify({
        enabled: true,
        upstream_error_enabled: true,
        upstream_error_trigger_percent: 50,
        upstream_error_min_samples: 10,
      }),
    );

    assert.equal('upstream_error_enabled' in normalized, false);
    assert.equal(normalized.upstream_error_trigger_percent, 50);
    assert.equal(normalized.upstream_error_min_samples, 10);
    assert.equal(
      getPressureCoolingValidationError({
        enabled: true,
        upstream_error_enabled: 'legacy value',
      }),
      null,
    );
  });

  test('round trips upstream-error thresholds including the trigger count', () => {
    const value = normalizePressureCooling({
      enabled: true,
      upstream_error_trigger_percent: 25,
      upstream_error_min_samples: 20,
      upstream_error_trigger_count: 7,
      condition_mode: 'all',
    });

    assert.deepEqual(JSON.parse(serializePressureCooling(value)), {
      enabled: true,
      frt_threshold_ms: null,
      trigger_percent: null,
      cooldown_seconds: null,
      observation_window_seconds: null,
      upstream_error_trigger_percent: 25,
      upstream_error_min_samples: 20,
      upstream_error_trigger_count: 7,
      condition_mode: 'all',
    });
    assert.deepEqual(
      parsePressureCooling(serializePressureCooling(value)),
      value,
    );
  });

  test('serializes values equal to the global defaults explicitly', () => {
    const serialized = JSON.parse(
      serializePressureCooling(
        normalizePressureCooling({
          enabled: true,
          upstream_error_trigger_percent: 50,
          upstream_error_min_samples: 10,
          upstream_error_trigger_count: 0,
          condition_mode: 'any',
        }),
      ),
    );

    assert.equal(serialized.upstream_error_trigger_percent, 50);
    assert.equal(serialized.upstream_error_min_samples, 10);
    assert.equal(serialized.upstream_error_trigger_count, 0);
    assert.equal('condition_mode' in serialized, false);
  });

  test('omits all nullable upstream-error fields when they are null', () => {
    const serialized = JSON.parse(
      serializePressureCooling({
        enabled: true,
        upstream_error_trigger_percent: null,
        upstream_error_min_samples: null,
        upstream_error_trigger_count: null,
      }),
    );

    assert.equal('upstream_error_trigger_percent' in serialized, false);
    assert.equal('upstream_error_min_samples' in serialized, false);
    assert.equal('upstream_error_trigger_count' in serialized, false);
  });

  test('rejects invalid upstream-error values before saving', () => {
    const invalidValues = [
      { trigger_percent: 0 },
      { trigger_percent: 101 },
      { trigger_percent: 2.5 },
      { upstream_error_trigger_percent: -1 },
      { upstream_error_trigger_percent: 101 },
      { upstream_error_min_samples: -1 },
      { upstream_error_min_samples: 2.5 },
      { upstream_error_trigger_count: -1 },
      { upstream_error_trigger_count: 2.5 },
      { condition_mode: 'sometimes' },
    ];

    for (const value of invalidValues) {
      assert.notEqual(getPressureCoolingValidationError(value), null, value);
      assert.equal(isPressureCoolingSaveAllowed(value), false, value);
    }
    assert.equal(
      getPressureCoolingValidationError({
        trigger_percent: 1,
        upstream_error_trigger_percent: 0,
        upstream_error_min_samples: 0,
        upstream_error_trigger_count: 0,
        condition_mode: 'any',
      }),
      null,
    );
  });

  test('rejects fractional upstream-error trigger percent', () => {
    assert.equal(
      getPressureCoolingValidationError({
        upstream_error_trigger_percent: 50.5,
      }),
      'upstream_error_trigger_percent',
    );
  });

  test('serializes an upstream-error count condition with otherwise empty fields', () => {
    const serialized = serializePressureCooling({
      upstream_error_trigger_count: 3,
    });

    assert.notEqual(serialized, '');
    assert.deepEqual(JSON.parse(serialized), {
      enabled: null,
      frt_threshold_ms: null,
      trigger_percent: null,
      cooldown_seconds: null,
      observation_window_seconds: null,
      upstream_error_trigger_count: 3,
    });
  });

  test('wires trigger conditions and all upstream inputs', () => {
    assert.match(editorSource, /t\('触发条件'\)/);
    assert.match(editorSource, /t\('条件组合'\)/);
    assert.match(editorSource, /t\('任一条件满足即冷却'\)/);
    assert.match(editorSource, /t\('两个条件同时满足才冷却'\)/);
    assert.match(editorSource, /t\('上游报错'\)/);
    assert.match(
      editorSource,
      /v\.upstream_error_trigger_percent > 0 \|\|\s*v\.upstream_error_trigger_count > 0/,
    );
    assert.match(editorSource, /t\('错误数阈值'\)/);
    assert.doesNotMatch(editorSource, /checked=\{v\.upstream_error_enabled\}/);
  });

  test('does not render condition combination until an upstream threshold is enabled', () => {
    assert.doesNotMatch(renderEditor({}), /条件组合/);
    assert.match(
      renderEditor({ upstream_error_trigger_percent: 1 }),
      /条件组合/,
    );
    assert.match(renderEditor({ upstream_error_trigger_count: 1 }), /条件组合/);
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
