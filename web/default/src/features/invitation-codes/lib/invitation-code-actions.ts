export const invitationCodesQueryKeys = {
  all: ['invitation-codes'] as const,
  lists: () => [...invitationCodesQueryKeys.all, 'list'] as const,
  list: (params: Record<string, unknown>) =>
    [...invitationCodesQueryKeys.lists(), params] as const,
  details: () => [...invitationCodesQueryKeys.all, 'detail'] as const,
  detail: (id: number) => [...invitationCodesQueryKeys.details(), id] as const,
  usages: (id: number) =>
    [...invitationCodesQueryKeys.all, 'usages', id] as const,
  userCodes: () => [...invitationCodesQueryKeys.all, 'user'] as const,
  userQuota: () => [...invitationCodesQueryKeys.all, 'user-quota'] as const,
}
