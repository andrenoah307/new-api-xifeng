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

const legalConsentKeys = [
  'user-agreement',
  'privacy-policy',
  'terms-of-service',
];

export function createLegalConsentFeedbackState() {
  return {
    invalidKeys: new Set(),
    shakingKeys: new Set(),
  };
}

export function reduceLegalConsentFeedbackState(state, event) {
  if (event.type === 'validation-requested') {
    const invalidKeys = new Set(
      legalConsentKeys.filter((key) => !event.agreed[key]),
    );
    return {
      invalidKeys,
      shakingKeys: new Set(invalidKeys),
    };
  }

  if (event.type === 'document-agreed') {
    const invalidKeys = new Set(state.invalidKeys);
    const shakingKeys = new Set(state.shakingKeys);
    invalidKeys.delete(event.key);
    shakingKeys.delete(event.key);
    return { invalidKeys, shakingKeys };
  }

  if (event.target !== event.currentTarget) return state;

  const shakingKeys = new Set(state.shakingKeys);
  shakingKeys.delete(event.key);
  return { ...state, shakingKeys };
}

export function getLegalConsentRowPresentation(isInvalid, isShaking) {
  const rowClassNames = [];
  if (isInvalid) rowClassNames.push('legal-consent-invalid');
  if (isShaking) rowClassNames.push('legal-consent-shake');

  return {
    rowClassName: rowClassNames.length ? rowClassNames.join(' ') : undefined,
    invalidDataValue: isInvalid ? 'true' : undefined,
    shakeDataValue: isShaking ? 'true' : undefined,
    checkboxAriaInvalid: isInvalid || undefined,
  };
}
