function canonicalAmount(value) {
  const text = String(value).trim();
  if (text.length > 128 || !/^\d+(?:\.\d{1,2})?$/.test(text)) return null;
  const [whole, fraction = ''] = text.split('.');
  return `${whole.replace(/^0+(?=\d)/, '')}.${fraction.padEnd(2, '0')}`;
}

export function filterInvoiceOrders(orders, search) {
  const query = search.trim().toLowerCase();
  if (!query) return orders;
  const amount = canonicalAmount(query);
  return orders.filter(order =>
    order.trade_no.toLowerCase().includes(query) ||
    (amount !== null && canonicalAmount(order.money) === amount),
  );
}
