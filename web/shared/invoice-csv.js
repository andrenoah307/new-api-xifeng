function csvField(value) {
  let text = String(value ?? '');
  if (text === '-') return text;
  if (/^[\s]*[=+@-]/u.test(text) || /^[\t\r\n]/u.test(text)) text = "'" + text;
  return /[,"\r\n]/u.test(text) ? '"' + text.replaceAll('"', '""') + '"' : text;
}

export function generateInvoiceCSV(items, serviceName) {
  const rows = [['工单ID', '电子邮箱', '数量', '单价', '金额合计', '公司信息', '应税服务名称', '备注']];
  for (const item of items) {
    rows.push([
      item.ticket_id, item.email, '', '', item.total_money.toFixed(2),
      item.tax_number ? `发票抬头\n${item.company_name}\n购方税号\n${item.tax_number}` : item.company_name,
      serviceName, item.remark ?? '',
    ]);
  }
  return '\uFEFF' + rows.map((row) => row.map(csvField).join(',')).join('\r\n');
}

export function generateFinanceInvoiceCSV(items, serviceName, refund = false) {
  const BOM = '﻿';
  const headers = [
    '工单ID',
    '电子邮箱',
    '数量',
    '单价',
    '金额合计',
    '公司信息',
    '应税服务名称',
    '发票类型',
    '发票税率(%)',
    '开票方（公账名称）',
  ];
  if (refund) {
    headers.splice(6, 1);
    headers.push('关联充值订单号', '发票类型（红字/蓝字）');
  }
  headers.push('备注');
  const lines = [headers.map(csvField).join(',')];
  for (const item of refund
    ? items.flatMap((i) =>
        (i.correction_rows ?? []).map((row) => ({
          ...row,
          ticket_id: i.ticket_id,
        })),
      )
    : items) {
    const companyInfo = item.tax_number
      ? `发票抬头\n${item.company_name}\n购方税号\n${item.tax_number}`
      : item.company_name;
    const issuers = item.issuer_amounts?.length
      ? item.issuer_amounts
      : [
          {
            account_name: '-',
            amount_cents: Math.round(item.total_money * 100),
          },
        ];
    for (const issuer of issuers) {
      const row = [
        item.ticket_id,
        item.email,
        '',
        '',
        (issuer.amount_cents / 100).toFixed(2),
        companyInfo,
        serviceName,
        ({ special: '增值税专用发票', ordinary: '增值税普通发票' })[item.invoice_type] ?? '',
        item.tax_rate_bps == null ? '' : item.tax_rate_bps / 100,
        issuer.account_name,
      ];
      if (refund) {
        row.splice(6, 1);
        row.push(
          (item.order_numbers ?? []).join(';'),
          item.invoice_color === 'red'
            ? '红字发票（需冲红）'
            : '蓝字发票（需重开）',
        );
      }
      row.push(item.remark ?? '');
      lines.push(row.map(csvField).join(','));
    }
  }
  return BOM + lines.join('\r\n');
}
