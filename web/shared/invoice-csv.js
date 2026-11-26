function csvField(value) {
  let text = String(value ?? '');
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
