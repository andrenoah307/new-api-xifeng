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
import { describe, test } from 'node:test';

import Checkbox from '@douyinfe/semi-ui/lib/es/checkbox/checkbox';
import Text from '@douyinfe/semi-ui/lib/es/typography/text';
import { renderToStaticMarkup } from 'react-dom/server';

import {
  createLegalConsentFeedbackState,
  getLegalConsentRowPresentation,
  reduceLegalConsentFeedbackState,
} from './legal-consent-shake.js';

const allUnchecked = {
  'user-agreement': false,
  'privacy-policy': false,
  'terms-of-service': false,
};

const validationCases = [
  {
    name: 'selects all three rows when every document is unchecked',
    agreed: allUnchecked,
    expected: ['user-agreement', 'privacy-policy', 'terms-of-service'],
  },
  {
    name: 'selects only unchecked rows when agreement is partial',
    agreed: {
      ...allUnchecked,
      'privacy-policy': true,
    },
    expected: ['user-agreement', 'terms-of-service'],
  },
  {
    name: 'selects no rows when every document is checked',
    agreed: {
      'user-agreement': true,
      'privacy-policy': true,
      'terms-of-service': true,
    },
    expected: [],
  },
];

describe('Classic legal consent shake state', () => {
  for (const item of validationCases) {
    test(item.name, () => {
      const nextState = reduceLegalConsentFeedbackState(
        createLegalConsentFeedbackState(),
        { type: 'validation-requested', agreed: item.agreed },
      );

      assert.deepEqual([...nextState.invalidKeys], item.expected);
      assert.deepEqual([...nextState.shakingKeys], item.expected);
      assert.notStrictEqual(nextState.invalidKeys, nextState.shakingKeys);
    });
  }

  const row = {};
  const child = {};
  const lifecycleCases = [
    {
      name: 'checking a document clears both invalid and shake state',
      event: { type: 'document-agreed', key: 'privacy-policy' },
      expectedInvalid: ['user-agreement', 'terms-of-service'],
      expectedShaking: ['user-agreement', 'terms-of-service'],
    },
    {
      name: 'the row animation ending clears only shake state',
      event: {
        type: 'animation-ended',
        key: 'privacy-policy',
        target: row,
        currentTarget: row,
      },
      expectedInvalid: ['user-agreement', 'privacy-policy', 'terms-of-service'],
      expectedShaking: ['user-agreement', 'terms-of-service'],
    },
    {
      name: 'a bubbled descendant animation does not clear either state',
      event: {
        type: 'animation-ended',
        key: 'privacy-policy',
        target: child,
        currentTarget: row,
      },
      expectedInvalid: ['user-agreement', 'privacy-policy', 'terms-of-service'],
      expectedShaking: ['user-agreement', 'privacy-policy', 'terms-of-service'],
    },
  ];

  for (const item of lifecycleCases) {
    test(item.name, () => {
      const validatedState = reduceLegalConsentFeedbackState(
        createLegalConsentFeedbackState(),
        { type: 'validation-requested', agreed: allUnchecked },
      );
      const nextState = reduceLegalConsentFeedbackState(
        validatedState,
        item.event,
      );

      assert.deepEqual([...nextState.invalidKeys], item.expectedInvalid);
      assert.deepEqual([...nextState.shakingKeys], item.expectedShaking);
    });
  }
});

function renderConsentRow(isInvalid, isShaking) {
  const presentation = getLegalConsentRowPresentation(isInvalid, isShaking);

  return renderToStaticMarkup(
    <div
      className={presentation.rowClassName}
      data-consent-invalid={presentation.invalidDataValue}
      data-consent-shake={presentation.shakeDataValue}
    >
      <Checkbox
        checked={false}
        aria-invalid={presentation.checkboxAriaInvalid}
        onChange={() => {}}
      >
        <Text size='small' className='text-gray-600'>
          Consent{' '}
          <a href='#' className='text-blue-600 hover:text-blue-800'>
            document
          </a>
        </Text>
      </Checkbox>
    </div>,
  );
}

describe('Classic legal consent row rendering', () => {
  test('marks a targeted row as invalid, red, and shaking', () => {
    const markup = renderConsentRow(true, true);

    assert.match(markup, /data-consent-invalid="true"/);
    assert.match(markup, /data-consent-shake="true"/);
    assert.match(markup, /legal-consent-invalid/);
    assert.match(markup, /legal-consent-shake/);
    assert.match(markup, /aria-invalid="true"/);
  });

  test('keeps the row invalid after its one-time shake has ended', () => {
    const markup = renderConsentRow(true, false);

    assert.match(markup, /data-consent-invalid="true"/);
    assert.match(markup, /legal-consent-invalid/);
    assert.match(markup, /aria-invalid="true"/);
    assert.doesNotMatch(markup, /data-consent-shake/);
    assert.doesNotMatch(markup, /legal-consent-shake/);
  });
});
