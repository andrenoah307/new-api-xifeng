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

import { splitFeaturedGroups } from './constants'

const groups = ['alpha', 'beta', 'gamma', 'delta'].map((group_name) => ({
  group_name,
}))

describe('splitFeaturedGroups', () => {
  // 契约：白名单分组置顶为单列宽卡片，其余分组留在下方多列网格。
  // 两侧必须互斥 —— 同一个分组同时出现在置顶区和下方网格里，管理员会以为
  // 监控重复统计了这个分组。
  test('lifts whitelisted groups out of the grid instead of duplicating them', () => {
    const { featured, rest } = splitFeaturedGroups(
      groups,
      new Set(['beta', 'delta'])
    )

    assert.deepEqual(
      featured.map((g) => g.group_name),
      ['beta', 'delta']
    )
    assert.deepEqual(
      rest.map((g) => g.group_name),
      ['alpha', 'gamma']
    )
  })

  // 置顶区和网格都必须沿用调用方给定的顺序，也就是管理员在
  // group_display_order 里配置的顺序（或用户选择的排序档），不能各自重排。
  test('preserves the caller order inside both sections', () => {
    const reversed = [...groups].reverse()
    const { featured, rest } = splitFeaturedGroups(
      reversed,
      new Set(['alpha', 'gamma'])
    )

    assert.deepEqual(
      featured.map((g) => g.group_name),
      ['gamma', 'alpha']
    )
    assert.deepEqual(
      rest.map((g) => g.group_name),
      ['delta', 'beta']
    )
  })

  // 白名单为空（管理员一个分组都没勾）时必须整体回落到多列网格，
  // 而不是留下一个空的置顶区把页面顶开一大块。
  test('falls back to a pure grid when nothing is whitelisted', () => {
    const { featured, rest } = splitFeaturedGroups(groups, new Set())

    assert.deepEqual(featured, [])
    assert.deepEqual(
      rest.map((g) => g.group_name),
      ['alpha', 'beta', 'gamma', 'delta']
    )
  })

  // 白名单里配了已被删除/被地区限制过滤掉的分组时，不能凭空造出卡片。
  test('ignores whitelisted names that are not in the visible list', () => {
    const { featured, rest } = splitFeaturedGroups(
      groups,
      new Set(['beta', 'ghost'])
    )

    assert.deepEqual(
      featured.map((g) => g.group_name),
      ['beta']
    )
    assert.equal(featured.length + rest.length, groups.length)
  })
})
