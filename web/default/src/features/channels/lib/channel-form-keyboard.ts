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
import type { KeyboardEvent as ReactKeyboardEvent } from 'react'

function hasOwnEnterBehavior(target: EventTarget | null): boolean {
  const element = target as HTMLElement | null
  if (!element) return false

  const tagName = element.tagName?.toLowerCase()
  if (tagName === 'textarea' || tagName === 'button') return true
  if (tagName === 'a') {
    if (
      typeof element.hasAttribute === 'function' &&
      element.hasAttribute('href')
    ) {
      return true
    }
    if (typeof element.hasAttribute !== 'function' && 'href' in element) {
      return true
    }
  }

  if (typeof element.closest !== 'function') {
    return Boolean(element.isContentEditable)
  }

  if (element.closest('textarea,button,a[href]')) return true

  const editableAncestor = element.closest('[contenteditable]')
  if (editableAncestor) {
    return (
      editableAncestor.getAttribute('contenteditable')?.trim().toLowerCase() !==
      'false'
    )
  }

  return Boolean(element.isContentEditable)
}

/**
 * Prevents the browser's implicit form submission when Enter is pressed in
 * channel editor fields. Child controls keep their own behavior by preventing
 * the event before it reaches this bubbling handler.
 */
export function handleChannelFormKeyDown(
  event: ReactKeyboardEvent<HTMLFormElement>
): void {
  if (event.defaultPrevented) return
  if (event.key !== 'Enter') return

  const nativeEvent = event.nativeEvent
  if (
    nativeEvent?.isComposing ||
    nativeEvent?.keyCode === 229 ||
    event.keyCode === 229
  ) {
    return
  }

  if (hasOwnEnterBehavior(event.target)) return
  event.preventDefault()
}
