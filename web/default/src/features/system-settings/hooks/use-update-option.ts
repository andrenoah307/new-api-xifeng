/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import {
  useMutation,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query'
import i18next from 'i18next'
import { toast } from 'sonner'

import { updateSystemOption } from '../api'
import type { UpdateOptionRequest } from '../types'

// Configuration keys that require status refresh
const STATUS_RELATED_KEYS = new Set([
  'theme.frontend',
  'HeaderNavModules',
  'SidebarModulesAdmin',
  'Notice',
  'LogConsumeEnabled',
  'QuotaPerUnit',
  'USDExchangeRate',
  'DisplayInCurrencyEnabled',
  'DisplayTokenStatEnabled',
  'general_setting.quota_display_type',
  'general_setting.custom_currency_symbol',
  'general_setting.custom_currency_exchange_rate',
  'region_restriction.enabled',
  'region_restriction.blocked_groups',
  'region_restriction.blocked_models',
  'region_restriction.filter_console',
  'group_model_blacklist.enabled',
  'group_model_blacklist.blocked_models',
  'RateLimitCapacityCardEnabled',
  'ModelNameRPMRateLimit',
  'console_setting.api_info_enabled',
  'console_setting.uptime_kuma_enabled',
  'console_setting.announcements_enabled',
  'console_setting.faq_enabled',
])

export function invalidateOptionQueries(
  queryClient: QueryClient,
  optionKey: string
): void {
  void queryClient.invalidateQueries({ queryKey: ['system-options'] })

  if (!STATUS_RELATED_KEYS.has(optionKey)) return

  void queryClient.invalidateQueries({ queryKey: ['status'] })
  try {
    window.localStorage.removeItem('status')
  } catch {
    /* empty */
  }
}

export function useUpdateOption() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (request: UpdateOptionRequest) => updateSystemOption(request),
    onSuccess: (data, variables) => {
      if (data.success) {
        invalidateOptionQueries(queryClient, variables.key)

        toast.success(i18next.t('Setting updated successfully'))
      } else {
        toast.error(data.message || i18next.t('Failed to update setting'))
      }
    },
    onError: (error: Error) => {
      toast.error(error.message || i18next.t('Failed to update setting'))
    },
  })
}
