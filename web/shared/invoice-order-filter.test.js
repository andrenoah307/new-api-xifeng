import { test } from 'node:test';
import assert from 'node:assert/strict';
import { filterInvoiceOrders } from './invoice-order-filter.js';

const orders = [
  { trade_no: 'USR6596NOXc338Y', money: 50 },
  { trade_no: 'OTHER', money: 150 },
  { trade_no: 'LARGE', money: '100000.01' },
  { trade_no: 'EXACT', money: '9007199254740993.01' },
];
test('invoice filtering matches partial order numbers and exact decimal amounts without mutating orders', () => {
  assert.deepEqual(filterInvoiceOrders(orders, '  noxc338y '), [orders[0]]);
  for (const value of ['50', '50.0', '050.00']) assert.deepEqual(filterInvoiceOrders(orders, value), [orders[0]]);
  assert.deepEqual(filterInvoiceOrders(orders, '100000.01'), [orders[2]]);
  assert.deepEqual(filterInvoiceOrders(orders, '9007199254740993.01'), [orders[3]]);
  for (const value of ['50.001', '5e1', '-50', 'missing']) assert.deepEqual(filterInvoiceOrders(orders, value), []);
  assert.equal(filterInvoiceOrders(orders, ' '), orders);
  assert.equal(orders.length, 4);
});
