import { createFileRoute, redirect } from '@tanstack/react-router'
import { CUSTOM_DEFAULT_SECTION } from '@/features/system-settings/custom/section-registry'

export const Route = createFileRoute(
  '/_authenticated/system-settings/custom/'
)({
  beforeLoad: () => {
    throw redirect({
      to: '/system-settings/custom/$section',
      params: { section: CUSTOM_DEFAULT_SECTION },
    })
  },
})
