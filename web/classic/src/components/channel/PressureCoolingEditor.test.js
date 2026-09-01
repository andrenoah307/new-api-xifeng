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

import {
  getPressureCoolingGroupOptions,
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
  });

  test('hides group selection outside the group scope', () => {
    assert.match(editorSource, /v\.scope === 'groups' && \(/);
    assert.match(
      editorSource,
      /optionList=\{getPressureCoolingGroupOptions\(groups\)\}/,
    );
  });
});
