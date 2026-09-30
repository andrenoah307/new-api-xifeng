import { useQuery } from '@tanstack/react-query'

import { getChannelCacheRates } from '../api'

export function useChannelCacheRates(enabled = true) {
  return useQuery({
    queryKey: ['channels', 'cache-rate-1h'],
    queryFn: getChannelCacheRates,
    enabled,
    staleTime: 30_000,
    refetchInterval: 60_000,
    retry: false,
  })
}
