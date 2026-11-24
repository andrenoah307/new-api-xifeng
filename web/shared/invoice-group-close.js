export async function previewInvoiceGroupClosures(post, convert) {
  const items = [];
  let after = 0;
  for (;;) {
    const { data } = await post('/api/option/invoice-tax/close-tickets/preview', { after_id: after, convert_order_tax: convert });
    if (!data.success) throw new Error(data.message);
    items.push(...data.data.items);
    if (!data.data.has_more) return items;
    if (data.data.after_id <= after) throw new Error('Invalid pagination');
    after = data.data.after_id;
  }
}

export async function closeInvoiceGroupTicket(post, item, convert) {
  const { data } = await post('/api/option/invoice-tax/close-tickets', { ticket_id: item.ticket_id, revision: item.revision, convert_order_tax: convert });
  if (!data.success) throw new Error(data.message);
  return data.data;
}

export function invoiceClosureStatus(item) {
  if (item.error) return item.error;
  return item.applied ? 'Closed' : 'Pending';
}
