import {
  Shield,
  Activity,
  Ticket,
  UserPlus,
  History,
  HeadsetIcon,
} from 'lucide-react'
import type { TFunction } from 'i18next'
import type { NavItem } from '@/components/layout/types'
import { ROLE } from '@/lib/roles'

export function getCustomAdminItems(t: TFunction): NavItem[] {
  return [
    {
      title: t('Risk Control'),
      url: '/risk',
      icon: Shield,
    },
    {
      title: t('Group Monitoring'),
      url: '/monitoring',
      icon: Activity,
    },
    {
      title: t('Invitation Codes'),
      url: '/invitation-codes',
      icon: UserPlus,
    },
  ]
}

export function getCustomGeneralItems(
  t: TFunction,
  role: number
): NavItem[] {
  const items: NavItem[] = [
    {
      title: t('Tickets'),
      url: '/tickets',
      icon: Ticket,
    },
    {
      title: t('Top-up History'),
      url: '/topup-history',
      icon: History,
    },
  ]

  if (role >= ROLE.TICKET_STAFF) {
    items.push({
      title: t('Ticket Admin'),
      url: '/ticket-admin',
      icon: HeadsetIcon,
    })
  }

  return items
}
