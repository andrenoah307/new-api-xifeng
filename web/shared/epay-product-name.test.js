import test from "node:test";
import assert from "node:assert/strict";
import {
  isValidEpayProductNameTemplate,
  previewEpayProductName,
} from "./epay-product-name.js";

test("Epay preview uses the same default and literal variables as checkout", () => {
  assert.equal(
    previewEpayProductName("", "充值"),
    "米醋API(demo_user)，充值10.00元",
  );
  assert.equal(
    previewEpayProductName("{username}/{amount}/{type}/{order_no}", "补税"),
    "demo_user/10.00/补税/DEMO123456",
  );
  assert.equal(isValidEpayProductNameTemplate(""), true);
  for (const invalid of [
    "{username}",
    "{amount}",
    "{username}{amount}{unknown}",
    "{{username}}{amount}",
    "{username}{amount}\n",
  ]) {
    assert.equal(isValidEpayProductNameTemplate(invalid), false, invalid);
  }
});
