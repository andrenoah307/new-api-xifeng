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
import type { LegalDocKey } from '@/features/auth/lib/legal-consent-storage'

const LEGAL_CONSENT_KEYS: readonly LegalDocKey[] = [
  'user-agreement',
  'privacy-policy',
  'terms-of-service',
]

export interface LegalConsentFeedbackState {
  invalidKeys: Set<LegalDocKey>
  shakingKeys: Set<LegalDocKey>
}

type LegalConsentFeedbackEvent =
  | {
      type: 'validation-requested'
      agreed: Readonly<Record<LegalDocKey, boolean>>
    }
  | { type: 'document-agreed'; key: LegalDocKey }
  | {
      type: 'animation-ended'
      key: LegalDocKey
      target: unknown
      currentTarget: unknown
    }

export function createLegalConsentFeedbackState(): LegalConsentFeedbackState {
  return {
    invalidKeys: new Set(),
    shakingKeys: new Set(),
  }
}

export function reduceLegalConsentFeedbackState(
  state: LegalConsentFeedbackState,
  event: LegalConsentFeedbackEvent
): LegalConsentFeedbackState {
  if (event.type === 'validation-requested') {
    const invalidKeys = new Set(
      LEGAL_CONSENT_KEYS.filter((key) => !event.agreed[key])
    )
    return {
      invalidKeys,
      shakingKeys: new Set(invalidKeys),
    }
  }

  if (event.type === 'document-agreed') {
    const invalidKeys = new Set(state.invalidKeys)
    const shakingKeys = new Set(state.shakingKeys)
    invalidKeys.delete(event.key)
    shakingKeys.delete(event.key)
    return { invalidKeys, shakingKeys }
  }

  if (event.target !== event.currentTarget) return state

  const shakingKeys = new Set(state.shakingKeys)
  shakingKeys.delete(event.key)
  return { ...state, shakingKeys }
}
