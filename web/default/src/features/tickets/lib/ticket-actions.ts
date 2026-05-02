export const ticketQueryKeys = {
  all: ['tickets'] as const,
  userLists: () => [...ticketQueryKeys.all, 'user-list'] as const,
  userList: (params: Record<string, unknown>) =>
    [...ticketQueryKeys.userLists(), params] as const,
  userDetail: (id: number) =>
    [...ticketQueryKeys.all, 'user-detail', id] as const,
  adminLists: () => [...ticketQueryKeys.all, 'admin-list'] as const,
  adminList: (params: Record<string, unknown>) =>
    [...ticketQueryKeys.adminLists(), params] as const,
  adminDetail: (id: number) =>
    [...ticketQueryKeys.all, 'admin-detail', id] as const,
  adminInvoice: (ticketId: number) =>
    [...ticketQueryKeys.all, 'admin-invoice', ticketId] as const,
  adminRefund: (ticketId: number) =>
    [...ticketQueryKeys.all, 'admin-refund', ticketId] as const,
  adminUserProfile: (ticketId: number) =>
    [...ticketQueryKeys.all, 'admin-profile', ticketId] as const,
  staff: () => [...ticketQueryKeys.all, 'staff'] as const,
  eligibleOrders: () => [...ticketQueryKeys.all, 'eligible-orders'] as const,
}
