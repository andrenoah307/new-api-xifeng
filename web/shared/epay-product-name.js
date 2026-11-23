export const DEFAULT_EPAY_PRODUCT_NAME_TEMPLATE =
  "米醋API({username})，{type}{amount}元";

export function isValidEpayProductNameTemplate(template) {
  if (template === "") return true;
  const remaining = template.replace(
    /\{(username|amount|order_no|type)\}/g,
    "",
  );
  return (
    [...template].length <= 200 &&
    template.includes("{username}") &&
    template.includes("{amount}") &&
    !/[{}\p{Cc}]/u.test(remaining)
  );
}

export function previewEpayProductName(template, type) {
  const values = {
    username: "demo_user",
    amount: "10.00",
    order_no: "DEMO123456",
    type,
  };
  return (template || DEFAULT_EPAY_PRODUCT_NAME_TEMPLATE).replace(
    /\{(username|amount|order_no|type)\}/g,
    (_, key) => values[key],
  );
}
