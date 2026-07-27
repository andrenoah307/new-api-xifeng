import { createContext, useContext, useState, useCallback } from 'react'
import type { InvitationCode } from '../api'

interface InvitationCodesContextValue {
  editingCode: InvitationCode | null
  sheetOpen: boolean
  openCreate: () => void
  openEdit: (code: InvitationCode) => void
  closeSheet: () => void
  usagesCode: InvitationCode | null
  usagesOpen: boolean
  openUsages: (code: InvitationCode) => void
  closeUsages: () => void
}

const InvitationCodesContext = createContext<InvitationCodesContextValue | null>(
  null
)

export function useInvitationCodes() {
  const ctx = useContext(InvitationCodesContext)
  if (!ctx)
    throw new Error(
      'useInvitationCodes must be used within InvitationCodesProvider'
    )
  return ctx
}

export function InvitationCodesProvider({
  children,
}: {
  children: React.ReactNode
}) {
  const [editingCode, setEditingCode] = useState<InvitationCode | null>(null)
  const [sheetOpen, setSheetOpen] = useState(false)
  const [usagesCode, setUsagesCode] = useState<InvitationCode | null>(null)
  const [usagesOpen, setUsagesOpen] = useState(false)

  const openCreate = useCallback(() => {
    setEditingCode(null)
    setSheetOpen(true)
  }, [])

  const openEdit = useCallback((code: InvitationCode) => {
    setEditingCode(code)
    setSheetOpen(true)
  }, [])

  const closeSheet = useCallback(() => {
    setSheetOpen(false)
    setTimeout(() => setEditingCode(null), 300)
  }, [])

  const openUsages = useCallback((code: InvitationCode) => {
    setUsagesCode(code)
    setUsagesOpen(true)
  }, [])

  const closeUsages = useCallback(() => {
    setUsagesOpen(false)
    setTimeout(() => setUsagesCode(null), 300)
  }, [])

  return (
    <InvitationCodesContext.Provider
      value={{
        editingCode,
        sheetOpen,
        openCreate,
        openEdit,
        closeSheet,
        usagesCode,
        usagesOpen,
        openUsages,
        closeUsages,
      }}
    >
      {children}
    </InvitationCodesContext.Provider>
  )
}
