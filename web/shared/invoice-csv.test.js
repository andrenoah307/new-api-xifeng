import assert from 'node:assert/strict';
import { test } from 'node:test';
import { generateInvoiceCSV, generateFinanceInvoiceCSV } from './invoice-csv.js';

test('invoice CSV appends escaped remarks and preserves previous columns', () => {
  const csv = generateInvoiceCSV([{ ticket_id: 7, email: 'a@b.com', total_money: 12.5, company_name: 'Company', tax_number: '', remark: 'A,"B"\nC' }], 'Service');
  assert.equal(csv, '\uFEFF工单ID,电子邮箱,数量,单价,金额合计,公司信息,应税服务名称,备注\r\n7,a@b.com,,,12.50,Company,Service,"A,""B""\nC"');
});

test('finance export preserves issuer amounts and appends safe remarks', () => {
  const item = { ticket_id: 8, email: 'a@b.com', total_money: 12, company_name: 'Company', tax_number: '', invoice_type: 'ordinary', tax_rate_bps: 600, remark: '=1+1', issuer_amounts: [{ account_name: 'Issuer A', amount_cents: 700 }, { account_name: 'Issuer B', amount_cents: 500 }] };
  const rows = generateFinanceInvoiceCSV([item], 'Service').split('\r\n');
  assert.equal(rows[0], '\uFEFF工单ID,电子邮箱,数量,单价,金额合计,公司信息,应税服务名称,发票类型,发票税率(%),开票方（公账名称）,备注');
  assert.equal(rows[1], "8,a@b.com,,,7.00,Company,Service,增值税普通发票,6,Issuer A,'=1+1");
  assert.equal(rows[2], "8,a@b.com,,,5.00,Company,Service,增值税普通发票,6,Issuer B,'=1+1");
  const refund = generateFinanceInvoiceCSV([{ ...item, correction_rows: [{ ...item, ticket_id: 99, order_numbers: ['order-1'], invoice_color: 'red' }] }], 'Service', true);
  assert.ok(refund.split('\r\n')[0].endsWith('关联充值订单号,发票类型（红字/蓝字）,备注'));
  assert.ok(refund.split('\r\n')[1].startsWith('8,a@b.com,,,7.00,Company,增值税普通发票'));
  assert.ok(refund.split('\r\n')[1].endsWith("Issuer A,order-1,红字发票（需冲红）,'=1+1"));
});

test('invoice CSV neutralizes formulas and supports missing legacy remarks', () => {
  const item = { ticket_id: 1, email: 'a@b.com', total_money: 1, company_name: 'Company', tax_number: '' };
  for (const remark of ['=1+1', '+SUM(A1)', '-1+1', '@SUM(A1)', '  =1', '\t=1']) {
    assert.ok(generateInvoiceCSV([{ ...item, remark }], 'Service').endsWith(",'" + remark));
  }
  assert.ok(generateInvoiceCSV([item], 'Service').endsWith('Service,'));
});
