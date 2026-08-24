export const INVOICE_SUMMARY_PREFIX = '发票申请信息：'
export const REFUND_SUMMARY_PREFIX = '退款申请信息：'
export const TICKET_REMARK_SEPARATOR = '\n备注：\n'

export function isGeneratedTicketSummary(
  content: string | undefined,
  ticketType: string
): boolean {
  if (!content) return false
  if (ticketType === 'invoice') {
    return content.startsWith(INVOICE_SUMMARY_PREFIX)
  }
  if (ticketType === 'refund') {
    return content.startsWith(REFUND_SUMMARY_PREFIX)
  }
  return false
}

export function extractRemarkFromSummary(content: string | undefined): string {
  const isGeneratedSummary =
    isGeneratedTicketSummary(content, 'invoice') ||
    isGeneratedTicketSummary(content, 'refund')
  if (!content || !isGeneratedSummary) return ''

  const separatorIndex = content.indexOf(TICKET_REMARK_SEPARATOR)
  if (separatorIndex === -1) return ''

  return content.slice(separatorIndex + TICKET_REMARK_SEPARATOR.length).trim()
}
