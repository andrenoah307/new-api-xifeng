import React, { useState } from 'react'
import useDialogState from '@/hooks/use-dialog'
import type {
  AutoGroupRule,
  AutoGroupEnrollment,
  AutoGroupDialogType,
} from '../types'

type AutoGroupContextType = {
  open: AutoGroupDialogType | null
  setOpen: (str: AutoGroupDialogType | null) => void
  currentRule: AutoGroupRule | null
  setCurrentRule: React.Dispatch<React.SetStateAction<AutoGroupRule | null>>
  currentEnrollment: AutoGroupEnrollment | null
  setCurrentEnrollment: React.Dispatch<
    React.SetStateAction<AutoGroupEnrollment | null>
  >
  refreshTrigger: number
  triggerRefresh: () => void
  activeTab: string
  setActiveTab: (tab: string) => void
}

const AutoGroupContext = React.createContext<AutoGroupContextType | null>(null)

export function AutoGroupProvider({ children }: { children: React.ReactNode }) {
  const [open, setOpen] = useDialogState<AutoGroupDialogType>(null)
  const [currentRule, setCurrentRule] = useState<AutoGroupRule | null>(null)
  const [currentEnrollment, setCurrentEnrollment] =
    useState<AutoGroupEnrollment | null>(null)
  const [refreshTrigger, setRefreshTrigger] = useState(0)
  const [activeTab, setActiveTab] = useState('rules')

  const triggerRefresh = () => setRefreshTrigger((prev) => prev + 1)

  return (
    <AutoGroupContext
      value={{
        open,
        setOpen,
        currentRule,
        setCurrentRule,
        currentEnrollment,
        setCurrentEnrollment,
        refreshTrigger,
        triggerRefresh,
        activeTab,
        setActiveTab,
      }}
    >
      {children}
    </AutoGroupContext>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export const useAutoGroup = () => {
  const context = React.useContext(AutoGroupContext)
  if (!context) {
    throw new Error('useAutoGroup must be used within <AutoGroupProvider>')
  }
  return context
}
