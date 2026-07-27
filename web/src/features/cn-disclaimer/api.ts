import { api } from '@/lib/api'
import type { CnDisclaimerResponse } from './types'

export async function getCnDisclaimer() {
  const res = await api.get<CnDisclaimerResponse>('/api/cn-disclaimer')
  return res.data
}
