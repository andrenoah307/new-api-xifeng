import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  extractRemarkFromSummary,
  isGeneratedTicketSummary,
} from './ticket-summary'

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
] as const

describe('ticket summary helpers', () => {
  for (const summaryCase of summaryCases) {
    test(summaryCase.name, () => {
      assert.equal(
        isGeneratedTicketSummary(summaryCase.content, summaryCase.ticketType),
        summaryCase.generated
      )
      assert.equal(
        extractRemarkFromSummary(summaryCase.content),
        summaryCase.remark
      )
    })
  }
})
