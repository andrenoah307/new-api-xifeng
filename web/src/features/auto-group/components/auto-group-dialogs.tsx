import { RuleMutateDrawer } from './rule-mutate-drawer'
import { RuleDeleteDialog } from './rule-delete-dialog'
import { EnrollDialog } from './enroll-dialog'
import { EnrollmentDeleteDialog } from './enrollment-delete-dialog'
import { useAutoGroup } from './auto-group-provider'

export function AutoGroupDialogs() {
  const { open, setOpen, currentRule } = useAutoGroup()
  const isUpdateRule = open === 'update-rule'

  return (
    <>
      <RuleMutateDrawer
        open={open === 'create-rule' || isUpdateRule}
        onOpenChange={(isOpen) => !isOpen && setOpen(null)}
        currentRow={isUpdateRule ? currentRule || undefined : undefined}
      />
      <RuleDeleteDialog />
      <EnrollDialog />
      <EnrollmentDeleteDialog />
    </>
  )
}
