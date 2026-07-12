import React, { useState } from 'react'
import useDialogState from '@/hooks/use-dialog'
import { type DiscountCode, type DiscountCodesDialogType } from '../types'

type DiscountCodesContextType = {
  open: DiscountCodesDialogType | null
  setOpen: (str: DiscountCodesDialogType | null) => void
  currentRow: DiscountCode | null
  setCurrentRow: React.Dispatch<React.SetStateAction<DiscountCode | null>>
  refreshTrigger: number
  triggerRefresh: () => void
}

const DiscountCodesContext =
  React.createContext<DiscountCodesContextType | null>(null)

export function DiscountCodesProvider({
  children,
}: {
  children: React.ReactNode
}) {
  const [open, setOpen] = useDialogState<DiscountCodesDialogType>(null)
  const [currentRow, setCurrentRow] = useState<DiscountCode | null>(null)
  const [refreshTrigger, setRefreshTrigger] = useState(0)

  const triggerRefresh = () => setRefreshTrigger((prev) => prev + 1)

  return (
    <DiscountCodesContext
      value={{
        open,
        setOpen,
        currentRow,
        setCurrentRow,
        refreshTrigger,
        triggerRefresh,
      }}
    >
      {children}
    </DiscountCodesContext>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export const useDiscountCodes = () => {
  const context = React.useContext(DiscountCodesContext)

  if (!context) {
    throw new Error(
      'useDiscountCodes has to be used within <DiscountCodesProvider>'
    )
  }

  return context
}
