import { DiscountCodesDeleteDialog } from './discount-codes-delete-dialog'
import { DiscountCodesMutateDrawer } from './discount-codes-mutate-drawer'
import { useDiscountCodes } from './discount-codes-provider'

export function DiscountCodesDialogs() {
  const { open, setOpen, currentRow } = useDiscountCodes()
  const isUpdate = open === 'update'

  return (
    <>
      <DiscountCodesMutateDrawer
        open={open === 'create' || isUpdate}
        onOpenChange={(isOpen) => !isOpen && setOpen(null)}
        currentRow={isUpdate ? currentRow || undefined : undefined}
      />
      <DiscountCodesDeleteDialog />
    </>
  )
}
