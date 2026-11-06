// Rational arithmetic preserves quota when an untouched monetary input is saved.
function decimalParts(value) {
  if (String(value).length > 128 || !/^\d+(\.\d{0,18})?$/.test(String(value))) return null;
  const [whole, fraction = ''] = String(value).split('.');
  return [BigInt(whole + fraction), 10n ** BigInt(fraction.length)];
}

export function createFinanceMoney(unit, rate) {
  const u = decimalParts(unit), r = decimalParts(rate);
  const valid = !!u && !!r && u[0] > 0n && r[0] > 0n;
  const numerator = valid ? r[0] * u[1] : 0n;
  const denominator = valid ? r[1] * u[0] : 0n;
  function parse(value) {
    const parts = decimalParts(value);
    if (!valid || !parts) return null;
    const n = parts[0] * denominator, d = parts[1] * numerator;
    const rounded = (n * 2n + d) / (2n * d);
    return rounded <= BigInt(Number.MAX_SAFE_INTEGER) ? Number(rounded) : null;
  }
  function input(value) {
    if (!valid || !Number.isSafeInteger(value) || value < 0) return '';
    for (let digits = 0; digits <= 18; digits++) {
      const scale = 10n ** BigInt(digits);
      const n = BigInt(value) * numerator * scale;
      const rounded = (n * 2n + denominator) / (2n * denominator);
      const text = digits ? `${rounded / scale}.${String(rounded % scale).padStart(digits, '0')}` : String(rounded);
      if (parse(text) === value) return text;
    }
    return '';
  }
  function format(value) {
    if (!valid || !/^-?\d+$/.test(String(value)) || (typeof value === 'number' && !Number.isSafeInteger(value))) return '-';
    const raw = BigInt(value), sign = raw < 0n ? '-' : '';
    const magnitude = raw < 0n ? -raw : raw;
    // Keep small nonzero differences visible while showing at least two decimals.
    const scale = 100000000n;
    const rounded = (magnitude * numerator * scale * 2n + denominator) / (2n * denominator);
    const fraction = String(rounded % scale).padStart(8, '0').replace(/0+$/, '').padEnd(2, '0');
    return `${sign}¥${rounded / scale}.${fraction}`;
  }
  return {valid, parse, input, format};
}
