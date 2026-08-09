import assert from 'node:assert/strict';
import { describe, test } from 'node:test';

import {
  TOKEN_PERIOD_MAX_DAYS,
  amountToCanonicalQuota,
  canonicalQuotaToAmount,
  canonicalQuotaToAmountInputString,
  canonicalQuotaToAmountString,
  convertPeriodLimitUnit,
  formatDisplayAmount,
  formatPeriodLimitValue,
  formatPeriodQuotaValue,
  formatPeriodResetAt,
  getPeriodBounds,
  getPeriodResetAt,
  isPositiveDecimalString,
  isPositiveIntegerString,
  normalizePeriodConversion,
  periodFormToPayload,
  periodResponseToForm,
  roundHalfAwayFromZero,
  validatePeriodForm,
} from './token-period';

const conversion = { displayRate: 7.3, quotaPerUnit: 500000, symbol: '¥' };

const fixture = [
  ['10.00', 7.3, 500000, 684932],
  ['0.01', 7.3, 500000, 685],
  ['1.00', 7.3, 500000, 68493],
  ['0.000001', 7.3, 500000, 0],
  ['30000', 7.3, 500000, 2054794521],
];

const canonicalQuotaInputFixture = [
  [0, '0'],
  [1, '0.00001'],
  [7, '0.0001'],
  [685, '0.01'],
  [12345, '0.18024'],
  [34247, '0.5'],
  [68493, '1'],
  [205479, '3'],
  [342466, '5'],
  [500000, '7.3'],
  [684932, '10'],
  [999999, '14.59999'],
  [6849315, '100'],
  [84558904, '1234.56'],
  [2054794521, '30000'],
  [2147483647, '31353.26125'],
];

describe('fixed amount quota conversion', () => {
  test('matches the Default fixture exactly', () => {
    for (const [amount, rate, quotaPerUnit, expected] of fixture) {
      assert.equal(
        amountToCanonicalQuota(amount, rate, quotaPerUnit),
        expected,
        `${amount}/${rate}/${quotaPerUnit}`,
      );
    }
  });

  test('uses decimal parsing and half-away-from-zero rounding', () => {
    assert.equal(roundHalfAwayFromZero('1.5'), 2);
    assert.equal(roundHalfAwayFromZero('-1.5'), -2);
    assert.equal(roundHalfAwayFromZero('1.49'), 1);
    assert.equal(amountToCanonicalQuota('1e-6', 1, 1000000), 1);
    assert.equal(amountToCanonicalQuota('1e6', 1, 1), 1000000);
    assert.equal(amountToCanonicalQuota('1', 0, 500000), 0);
    assert.equal(amountToCanonicalQuota('1', -1, 500000), -500000);
    assert.equal(amountToCanonicalQuota('bad', 7.3, 500000), 0);
    assert.equal(amountToCanonicalQuota('1e1001', 1, 1), 0);
    assert.equal(isPositiveDecimalString('0.01'), true);
    assert.equal(isPositiveDecimalString('0'), false);
    assert.equal(isPositiveDecimalString('-1'), false);
    assert.equal(isPositiveDecimalString('NaN'), false);
    assert.equal(isPositiveIntegerString('01'), true);
    assert.equal(isPositiveIntegerString('1.0'), false);
    assert.equal(isPositiveIntegerString('0'), false);
  });

  test('converts canonical quota back without following display mode', () => {
    assert.equal(canonicalQuotaToAmount(684932, 7.3, 500000), 10.0000072);
    assert.equal(
      canonicalQuotaToAmountString(684932, 7.3, 500000),
      '10.0000072',
    );
    assert.equal(canonicalQuotaToAmountString(0, 7.3, 500000), '0');
    assert.equal(canonicalQuotaToAmountString(10, 0, 500000), '0');
  });

  test('keeps the canonical value stable while switching units', () => {
    const quota = convertPeriodLimitUnit(
      '10.00',
      'cny',
      'quota',
      conversion,
    );
    const amount = convertPeriodLimitUnit(
      quota.value,
      'quota',
      'cny',
      conversion,
      quota.canonicalQuota,
    );
    const quotaAgain = convertPeriodLimitUnit(
      amount.value,
      'cny',
      'quota',
      conversion,
      amount.canonicalQuota,
    );
    assert.deepEqual(quota, { value: '684932', canonicalQuota: 684932 });
    assert.deepEqual(amount, {
      value: '10',
      canonicalQuota: 684932,
    });
    assert.deepEqual(quotaAgain, quota);
    assert.deepEqual(
      normalizePeriodConversion({ displayRate: 0, quotaPerUnit: -1 }),
      { displayRate: 1, quotaPerUnit: 1, symbol: '' },
    );
  });
});

describe('period amount follows the site display currency', () => {
  // 1 USD = 1 展示币种：金额刻度必须与令牌额度字段完全一致
  test('USD display keeps the amount on the same scale as the token quota', () => {
    assert.equal(amountToCanonicalQuota('300', 1, 500000), 150000000);
    assert.equal(canonicalQuotaToAmountInputString(150000000, 1, 500000), '300');
    assert.equal(
      formatPeriodLimitValue(
        150000000,
        'cny',
        { displayRate: 1, quotaPerUnit: 500000, symbol: '$' },
        'en-US',
      ),
      '$300.00',
    );
  });

  // TOKENS 口径编码成 displayRate === quotaPerUnit + 空符号，换算退化成恒等
  test('TOKENS display collapses the conversion to identity', () => {
    const tokens = { displayRate: 500000, quotaPerUnit: 500000, symbol: '' };
    assert.equal(amountToCanonicalQuota('300', 500000, 500000), 300);
    assert.equal(canonicalQuotaToAmountInputString(300, 500000, 500000), '300');
    assert.equal(formatPeriodLimitValue(684932, 'cny', tokens, 'en-US'), '684,932');
    assert.equal(formatPeriodQuotaValue(684932, 'cny', tokens, 'en-US'), '684,932');
  });

  test('custom currency uses the admin symbol and rate', () => {
    const custom = { displayRate: 3.5, quotaPerUnit: 500000, symbol: '¤' };
    assert.equal(amountToCanonicalQuota('7', 3.5, 500000), 1000000);
    assert.equal(
      formatPeriodLimitValue(1000000, 'cny', custom, 'en-US'),
      '¤7.00',
    );
  });

  test('formats an amount with or without a display symbol', () => {
    assert.equal(formatDisplayAmount('10', '¥', 'en-US'), '¥10.00');
    assert.equal(formatDisplayAmount('10.000007', '¥', 'en-US'), '¥10.000007');
    assert.equal(formatDisplayAmount('684932', '', 'en-US'), '684,932');
    assert.equal(formatDisplayAmount('bad', '¥', 'en-US'), '-');
  });
});

describe('zero period limit disables the period', () => {
  test('treats blank and zero limits as disabled in the payload', () => {
    for (const value of ['0', '', '  ', '0.00']) {
      assert.deepEqual(
        periodFormToPayload({
          period_enabled: true,
          period_type: 'month',
          period_days: 0,
          period_limit_unit: 'cny',
          period_limit_value: value,
        }),
        {
          period_type: '',
          period_days: 0,
          period_limit_unit: 'cny',
          period_limit_value: '0',
        },
        `value=${JSON.stringify(value)}`,
      );
    }
  });

  test('accepts a zero limit instead of blocking the form', () => {
    assert.deepEqual(
      validatePeriodForm(
        {
          period_enabled: true,
          period_type: 'days',
          period_days: 0,
          period_limit_unit: 'cny',
          period_limit_value: '0',
        },
        conversion,
      ),
      { valid: true, errors: [] },
    );
  });
});

describe('shortest amount input representatives', () => {
  test('returns the shortest decimal string for every canonical quota fixture', () => {
    for (const [quota, expected] of canonicalQuotaInputFixture) {
      const result = canonicalQuotaToAmountInputString(
        quota,
        conversion.displayRate,
        conversion.quotaPerUnit,
      );
      assert.equal(result, expected);
      assert.equal(
        amountToCanonicalQuota(
          result,
          conversion.displayRate,
          conversion.quotaPerUnit,
        ),
        quota,
      );
      assert.match(result, /^-?\d+(\.\d*[1-9])?$/);
      const decimalDigits = result.includes('.')
        ? result.length - result.indexOf('.') - 1
        : 0;
      if (decimalDigits > 0) {
        assert.notEqual(
          amountToCanonicalQuota(
            canonicalQuotaToAmountString(
              quota,
              conversion.displayRate,
              conversion.quotaPerUnit,
              decimalDigits - 1,
            ),
            conversion.displayRate,
            conversion.quotaPerUnit,
          ),
          quota,
        );
      }
    }
  });

  test('keeps representative strings stable through quantization boundaries', () => {
    assert.equal(
      canonicalQuotaToAmountInputString(
        amountToCanonicalQuota('0.000015', 7.3, 500000),
        7.3,
        500000,
      ),
      '0.00001',
    );
    assert.equal(amountToCanonicalQuota('0.000015', 7.3, 500000), 1);
    assert.equal(amountToCanonicalQuota('0.0000073', 7.3, 500000), 1);
    assert.equal(amountToCanonicalQuota('0.0000072999', 7.3, 500000), 0);
    assert.equal(amountToCanonicalQuota('0.0000219', 7.3, 500000), 2);
    assert.equal(amountToCanonicalQuota('0.0000218999', 7.3, 500000), 1);
  });

  test('supports alternate conversion parameters and invalid inputs', () => {
    assert.equal(canonicalQuotaToAmountInputString(1, 1, 2), '0.5');
    assert.equal(canonicalQuotaToAmountInputString(1, 0.25, 1), '0.3');
    assert.equal(
      canonicalQuotaToAmountInputString(68493, 1, 500000),
      '0.136986',
    );
    assert.equal(canonicalQuotaToAmountInputString(10, 0, 500000), '0');
    assert.equal(canonicalQuotaToAmountInputString(10, 7.3, 0), '0');
    assert.equal(canonicalQuotaToAmountInputString('bad', 7.3, 500000), '0');
  });

  test('uses the input representative for period limits but center values for usage', () => {
    assert.equal(
      formatPeriodLimitValue(68493, 'cny', conversion, 'en-US'),
      '¥1.00',
    );
    assert.equal(
      formatPeriodQuotaValue(1, 'cny', conversion, 'en-US'),
      '¥0.000015',
    );
    assert.equal(formatPeriodLimitValue(12, 'quota', conversion, 'en-US'), '12');
    assert.equal(formatPeriodLimitValue(NaN, 'cny', conversion, 'en-US'), '-');
  });

  test('keeps canonical quota stable across repeated unit switches', () => {
    const quota = convertPeriodLimitUnit('1', 'cny', 'quota', conversion);
    const amount = convertPeriodLimitUnit(
      quota.value,
      'quota',
      'cny',
      conversion,
      quota.canonicalQuota,
    );
    const quotaAgain = convertPeriodLimitUnit(
      amount.value,
      'cny',
      'quota',
      conversion,
      amount.canonicalQuota,
    );
    assert.deepEqual(quota, { value: '68493', canonicalQuota: 68493 });
    assert.deepEqual(amount, { value: '1', canonicalQuota: 68493 });
    assert.deepEqual(quotaAgain, quota);
  });

  test('hydrates the shortest amount period limit representative', () => {
    const form = periodResponseToForm(
      {
        period_type: 'month',
        period_quota_limit: 68493,
        period_limit_unit: 'cny',
      },
      conversion,
    );
    assert.equal(form.period_limit_value, '1');
    assert.equal(form.canonicalQuota, 68493);
  });
});

describe('period form contract', () => {
  test('normalizes disabled values and never includes server-owned fields', () => {
    const disabled = periodFormToPayload({
      period_enabled: false,
      period_type: 'days',
      period_days: 99,
      period_limit_unit: 'quota',
      period_limit_value: '123',
      period_used_quota: 4,
      period_start_at: 5,
      period_anchor_at: 6,
      period_reset_at: 7,
    });
    assert.deepEqual(disabled, {
      period_type: '',
      period_days: 0,
      period_limit_unit: 'quota',
      period_limit_value: '0',
    });
    for (const key of [
      'period_used_quota',
      'period_start_at',
      'period_anchor_at',
      'period_reset_at',
    ]) {
      assert.equal(key in disabled, false);
    }

    const enabled = periodFormToPayload({
      period_enabled: true,
      period_type: 'days',
      period_days: 3,
      period_limit_unit: 'cny',
      period_limit_value: '10.00',
    });
    assert.deepEqual(enabled, {
      period_type: 'days',
      period_days: 3,
      period_limit_unit: 'cny',
      period_limit_value: '10.00',
    });
  });

  test('hydrates the writable display value from canonical response fields', () => {
    const amount = periodResponseToForm(
      {
        period_type: 'week',
        period_days: 99,
        period_quota_limit: 684932,
        period_limit_unit: 'cny',
        period_reset_at: 1800000000,
      },
      conversion,
    );
    assert.deepEqual(amount, {
      period_enabled: true,
      period_type: 'week',
      period_days: 0,
      period_limit_unit: 'cny',
      period_limit_value: '10',
      period_reset_at: 1800000000,
      canonicalQuota: 684932,
      period_anchor_at: 0,
    });

    const quota = periodResponseToForm(
      {
        period_type: 'month',
        period_quota_limit: 123456,
        period_limit_unit: 'quota',
      },
      conversion,
    );
    assert.equal(quota.period_limit_value, '123456');
    assert.equal(quota.period_days, 0);

    const disabled = periodResponseToForm(
      { period_type: '', period_quota_limit: 0 },
      conversion,
    );
    assert.equal(disabled.period_enabled, false);
    assert.equal(disabled.period_limit_value, '0');
  });

  test('validates conditional days and unit values', () => {
    const base = {
      period_enabled: true,
      period_type: 'week',
      period_days: 0,
      period_limit_unit: 'cny',
      period_limit_value: '10.00',
    };
    assert.deepEqual(validatePeriodForm({ ...base }), {
      valid: true,
      errors: [],
    });
    assert.equal(
      validatePeriodForm({ ...base, period_type: 'days', period_days: 0 }).valid,
      false,
    );
    assert.equal(
      validatePeriodForm({
        ...base,
        period_type: 'days',
        period_days: TOKEN_PERIOD_MAX_DAYS + 1,
      }).valid,
      false,
    );
    assert.equal(
      validatePeriodForm({
        ...base,
        period_limit_unit: 'quota',
        period_limit_value: '1.5',
      }).valid,
      false,
    );
    assert.equal(
      validatePeriodForm({
        ...base,
        period_limit_unit: 'quota',
        period_limit_value: '123',
      }).valid,
      true,
    );
    // 超过 int32 的限额仍必须被拒绝
    assert.equal(
      validatePeriodForm(
        { ...base, period_limit_value: '31353.27' },
        conversion,
      ).valid,
      false,
    );
  });
});

describe('period boundaries and display', () => {
  const monday = Date.UTC(2026, 7, 3, 0, 0, 0) - 8 * 60 * 60 * 1000;
  const sunday = monday + 6 * 24 * 60 * 60 * 1000 + 23 * 60 * 60 * 1000;

  test('uses a fixed UTC+8 anchor for N-day periods', () => {
    const anchor = Math.floor(
      (Date.UTC(2026, 7, 1, 0, 0, 0) - 8 * 60 * 60 * 1000) / 1000,
    );
    assert.deepEqual(getPeriodBounds('days', 2, monday, anchor), {
      startAt: Math.floor(monday / 1000),
      resetAt: Math.floor((monday + 2 * 24 * 60 * 60 * 1000) / 1000),
    });
    assert.equal(getPeriodResetAt('days', 0, monday, anchor), 0);
    assert.equal(getPeriodResetAt('days', 3651, monday, anchor), 0);
  });

  test('uses Monday and natural-month boundaries', () => {
    assert.equal(
      getPeriodResetAt('week', 0, sunday),
      Math.floor((monday + 7 * 24 * 60 * 60 * 1000) / 1000),
    );
    assert.equal(
      getPeriodResetAt('week', 0, monday),
      Math.floor((monday + 7 * 24 * 60 * 60 * 1000) / 1000),
    );
    const monthEnd = Date.UTC(2026, 9, 1, 0, 0, 0) / 1000 - 8 * 60 * 60;
    assert.equal(
      getPeriodResetAt('month', 0, Date.UTC(2026, 7, 31, 16, 0, 0)),
      monthEnd,
    );
    assert.equal(getPeriodResetAt('', 0, monday), 0);
    assert.equal(getPeriodResetAt('invalid', 0, monday), 0);
  });

  test('formats both units and reset timestamps', () => {
    const resetAt = Math.floor(Date.UTC(2026, 7, 3, 16, 0, 0) / 1000);
    assert.match(formatPeriodResetAt(resetAt, 'en-US'), /UTC\+8/);
    assert.equal(formatPeriodResetAt(0, 'en-US'), '-');
    assert.equal(
      formatPeriodQuotaValue(684932, 'cny', conversion, 'en-US'),
      '¥10.000007',
    );
    assert.equal(
      formatPeriodQuotaValue(684932, 'quota', conversion, 'en-US'),
      '684,932',
    );
  });
});
