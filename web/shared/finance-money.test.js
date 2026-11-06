import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createFinanceMoney } from './finance-money.js';

test('CNY conversion uses configured unit and exchange rate, including signed differences', () => {
  const money = createFinanceMoney('500000', '7');
  assert.equal(money.format(672170000), '¥9410.38');
  assert.equal(money.format(0), '¥0.00');
  assert.equal(money.format('22000000'), '¥308.00');
  assert.equal(money.format('-22000000'), '-¥308.00');
  assert.equal(createFinanceMoney('100', '1').format(1234), '¥12.34');
});
test('editable monetary values round-trip without losing quota or cash cents', () => {
  for (const [unit,rate] of [['500000','7'],['500000','7.3'],['123456.789','6.88'],['100','1']]) {
    const money=createFinanceMoney(unit,rate);
    for(const quota of [0,1,68493,672170000,22000000]) assert.equal(money.parse(money.input(quota)),quota);
  }
  assert.equal(createFinanceMoney('500000','7').parse('308'),22000000);
});
test('missing configuration and invalid amounts cannot silently change accounting', () => {
  const missing=createFinanceMoney('', '7');
  assert.equal(missing.valid,false);
  assert.equal(missing.format(500000),'-');
  assert.equal(missing.parse('7'),null);
  const money=createFinanceMoney('500000','7');
  for(const value of ['', '-1','NaN','Infinity','1e100','1000000000000000000000']) assert.equal(money.parse(value),null);
});
