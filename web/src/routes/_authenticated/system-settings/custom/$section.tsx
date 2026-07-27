import { createFileRoute, redirect } from '@tanstack/react-router'
import { CustomSettings } from '@/features/system-settings/custom'
import {
  CUSTOM_DEFAULT_SECTION,
  CUSTOM_SECTION_IDS,
} from '@/features/system-settings/custom/section-registry'

export const Route = createFileRoute(
  '/_authenticated/system-settings/custom/$section'
)({
  beforeLoad: ({ params }) => {
    const validSections = CUSTOM_SECTION_IDS as unknown as string[]
    if (!validSections.includes(params.section)) {
      throw redirect({
        to: '/system-settings/custom/$section',
        params: { section: CUSTOM_DEFAULT_SECTION },
      })
    }
  },
  component: CustomSettings,
})
