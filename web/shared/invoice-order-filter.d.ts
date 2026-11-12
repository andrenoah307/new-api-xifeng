export function filterInvoiceOrders<T extends { trade_no: string; money: number | string }>(orders: T[], search: string): T[];
