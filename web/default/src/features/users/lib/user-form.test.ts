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

import {
  USER_FORM_DEFAULT_VALUES,
  transformFormDataToPayload,
  userFormSchema,
} from './user-form'

// 后端 UpdateUser 把缺席的 email 视为保持不变、空串视为解绑；
// 编辑提交必须始终携带表单中的邮箱，否则保存其它字段会误删绑定。
describe('user form email', () => {
  test('update payload carries the bound email', () => {
    const payload = transformFormDataToPayload(
      { ...USER_FORM_DEFAULT_VALUES, username: 'u', email: ' a@b.com ' },
      7
    )
    assert.equal(payload.email, 'a@b.com')
  })

  test('update payload sends empty string when cleared', () => {
    const payload = transformFormDataToPayload(
      { ...USER_FORM_DEFAULT_VALUES, username: 'u', email: '' },
      7
    )
    assert.equal(payload.email, '')
  })

  test('create payload omits email', () => {
    const payload = transformFormDataToPayload({
      ...USER_FORM_DEFAULT_VALUES,
      username: 'u',
      email: 'a@b.com',
    })
    assert.equal('email' in payload, false)
  })

  test('schema accepts empty and rejects malformed email', () => {
    const base = { ...USER_FORM_DEFAULT_VALUES, username: 'u' }
    assert.equal(userFormSchema.safeParse({ ...base, email: '' }).success, true)
    assert.equal(
      userFormSchema.safeParse({ ...base, email: 'not-an-email' }).success,
      false
    )
  })
})
