import assert from 'node:assert/strict';
import { test } from 'node:test';
import { generateInvoiceCSV } from './invoice-csv.js';

test('invoice CSV appends escaped remarks and preserves previous columns', () => {
  const csv = generateInvoiceCSV([{ ticket_id: 7, email: 'a@b.com', total_money: 12.5, company_name: 'Company', tax_number: '', remark: 'A,"B"\nC' }], 'Service');
  assert.equal(csv, '\uFEFF工单ID,电子邮箱,数量,单价,金额合计,公司信息,应税服务名称,备注\r\n7,a@b.com,,,12.50,Company,Service,"A,""B""\nC"');
});

test('invoice CSV neutralizes formulas and supports missing legacy remarks', () => {
  const item = { ticket_id: 1, email: 'a@b.com', total_money: 1, company_name: 'Company', tax_number: '' };
  for (const remark of ['=1+1', '+SUM(A1)', '-1+1', '@SUM(A1)', '  =1', '\t=1']) {
    assert.ok(generateInvoiceCSV([{ ...item, remark }], 'Service').endsWith(",'" + remark));
  }
  assert.ok(generateInvoiceCSV([item], 'Service').endsWith('Service,'));
});
