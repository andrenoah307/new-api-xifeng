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

import {
  extractRemarkFromSummary,
  isGeneratedTicketSummary,
} from './ticketUtils.js';

const summaryCases = [
  {
    name: 'extracts an invoice remark',
    content: '发票申请信息：\n公司名称：示例公司\n备注：\n用于项目报销',
    ticketType: 'invoice',
    generated: true,
    remark: '用于项目报销',
  },
  {
    name: 'returns an empty remark when the summary has no remark section',
    content: '发票申请信息：\n公司名称：示例公司',
    ticketType: 'invoice',
    generated: true,
    remark: '',
  },
  {
    name: 'preserves internal newlines in a multiline remark',
    content: '退款申请信息：\n退款额度：100\n备注：\n 第一行\n第二行 ',
    ticketType: 'refund',
    generated: true,
    remark: '第一行\n第二行',
  },
  {
    name: 'ignores a non-summary message containing the separator',
    content: '用户消息\n备注：\n不应提取',
    ticketType: 'invoice',
    generated: false,
    remark: '',
  },
  {
    name: 'handles undefined content',
    content: undefined,
    ticketType: 'invoice',
    generated: false,
    remark: '',
  },
  {
    name: 'requires the invoice summary colon',
    content: '发票申请信息\n备注：\n不应提取',
    ticketType: 'invoice',
    generated: false,
    remark: '',
  },
  {
    name: 'requires the summary prefix at the start',
    content: '这是发票申请信息：\n备注：\n不应提取',
    ticketType: 'invoice',
    generated: false,
    remark: '',
  },
];

describe('ticket summary helpers', () => {
  for (const summaryCase of summaryCases) {
    test(summaryCase.name, () => {
      assert.equal(
        isGeneratedTicketSummary(summaryCase.content, summaryCase.ticketType),
        summaryCase.generated,
      );
      assert.equal(
        extractRemarkFromSummary(summaryCase.content),
        summaryCase.remark,
      );
    });
  }
});
