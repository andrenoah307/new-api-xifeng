import assert from "node:assert/strict";
import { test } from "node:test";
import { readFileSync, readdirSync } from "node:fs";
import { refundDiagnosticLines } from "./refund-diagnostics.mjs";

const translate = (key, values = {}) => key.replaceAll(/\{\{(\w+)\}\}/g, (_, name) => values[name]);

test("finance sees a plain balance explanation, with exact values available", () => {
  const diagnostic = {
    code: "grace_exceeded",
    amounts: {
      recharged: "151",
      consumed: "120.848446",
      refunded: "0",
      available: "30.151554",
      requested: "50",
      shortfall: "19.848446",
      wallet: "0.151554",
      limit: "10",
    },
  };
  const lines = refundDiagnosticLines(diagnostic, translate);
  assert.match(lines[0], /151.00.*120.85.*0.00/);
  assert.match(lines[1], /30.15.*50.00.*19.85/);
  assert.match(lines[2], /0.15.*10.00/);
  assert.match(lines[3], /exceeds.*Manual review/);
  assert.match(refundDiagnosticLines(diagnostic, translate, true)[1], /19.848446/);
});

test("unbacked credit is not described as wallet debt", () => {
  const lines = refundDiagnosticLines({ code: "unbacked_credit" }, translate);
  assert.match(lines[0], /not covered by actual wallet debt/);
  assert.match(lines[0], /administrator credits/);
});

test("a sub-cent shortfall is never displayed as zero", () => {
  const lines = refundDiagnosticLines(
    {
      code: "grace_unavailable",
      amounts: {
        recharged: "1",
        consumed: "0.000002",
        refunded: "0",
        available: "0.999998",
        requested: "1",
        shortfall: "0.000002",
        wallet: "-0.000002",
        limit: "0",
      },
    },
    translate,
  );
  assert.match(lines[1], /shortfall 0.000002/);
  assert.match(lines[2], /balance -0.000002/);
});

test("missing history explains the next action without inventing amounts", () => {
  const lines = refundDiagnosticLines({ code: "missing_history", ticket_id: 99 }, translate);
  assert.equal(lines.length, 1);
  assert.match(lines[0], /Reconcile the related refund/);
  assert.doesNotMatch(lines[0], /undefined|NaN/);
});

test("rounding cannot hide a shortfall just over the grace limit", () => {
  const lines = refundDiagnosticLines(
    {
      code: "grace_exceeded",
      amounts: {
        recharged: "50",
        consumed: "10.000002",
        refunded: "0",
        available: "39.999998",
        requested: "50",
        shortfall: "10.000002",
        wallet: "-10.000002",
        limit: "10",
      },
    },
    translate,
  );
  assert.match(lines[1], /shortfall 10.000002/);
  assert.match(lines[2], /balance -10.000002/);
});

test("unknown diagnostic codes use a readable fallback", () => {
  assert.match(refundDiagnosticLines({ code: "future_reason" }, translate)[0], /Retry/);
});

test("both themes translate all diagnostic messages and retain amount placeholders", () => {
  const keys = new Set(["Order ID", "Exact amounts"]);
  for (const code of [
    "grace_exceeded",
    "grace_unavailable",
    "unbacked_credit",
    "hold_not_eligible",
    "gift_balance",
    "funding_limit",
    "missing_history",
    "completed_without_snapshot",
    "later_refund",
    "payment_evidence",
    "invalid_order",
    "allocation_mismatch",
    "payment_allocation",
    "invoice_pending",
    "duplicate_invoice",
    "invoice_allocation",
    "calculation_failed",
  ]) {
    refundDiagnosticLines({ code, amounts: {} }, (key) => {
      keys.add(key);
      return key;
    });
  }
  const placeholders = (text) => [...text.matchAll(/\{\{\w+\}\}/g)].map((match) => match[0]).sort();
  for (const theme of ["default", "classic"]) {
    const directory = new URL(`../${theme}/src/i18n/locales/`, import.meta.url);
    for (const file of readdirSync(directory).filter((file) => file.endsWith(".json"))) {
      const values = JSON.parse(readFileSync(new URL(file, directory), "utf8")).translation;
      for (const key of keys) {
        assert.equal(typeof values[key], "string", `${theme}/${file}: ${key}`);
        assert(values[key].trim(), `${theme}/${file}: empty translation`);
        assert.deepEqual(placeholders(values[key]), placeholders(key), `${theme}/${file}: ${key}`);
      }
    }
  }
});
