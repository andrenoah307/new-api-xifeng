import { useState, useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { manageUser } from '../../api'
import { USER_ROLE, ROLE_META } from '../../constants'
import type { User } from '../../types'

interface RoleManagementDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  user: User | null
  onSuccess: () => void
}

export function RoleManagementDialog({
  open,
  onOpenChange,
  user,
  onSuccess,
}: RoleManagementDialogProps) {
  const { t } = useTranslation()
  const myRole = useAuthStore((s) => s.auth.user?.role ?? 1)
  const [targetRole, setTargetRole] = useState<string>('')
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (open && user) {
      setTargetRole(String(user.role))
    }
  }, [open, user])

  const isRootUser = user?.role === USER_ROLE.ROOT

  const roleOptions = useMemo(() => {
    const all = [USER_ROLE.USER, USER_ROLE.STAFF, USER_ROLE.ADMIN, USER_ROLE.ROOT]
    return all.filter((r) => (myRole === USER_ROLE.ROOT ? true : r < myRole))
  }, [myRole])

  const handleConfirm = async () => {
    if (!user || targetRole === String(user.role)) return
    setLoading(true)
    try {
      const result = await manageUser(user.id, 'set_role', Number(targetRole))
      if (result.success) {
        toast.success(t('Role updated'))
        onSuccess()
        onOpenChange(false)
      } else {
        toast.error(result.message || t('Operation failed'))
      }
    } catch {
      toast.error(t('Operation failed'))
    } finally {
      setLoading(false)
    }
  }

  if (!user) return null

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg w-[560px] max-w-[92vw]">
        <DialogHeader>
          <DialogTitle>{t('Manage Role')}</DialogTitle>
        </DialogHeader>

        {isRootUser ? (
          <Alert variant="destructive">
            <AlertTitle>{t('Cannot modify Super Admin')}</AlertTitle>
            <AlertDescription>
              {t('Super Admin role can only be adjusted in the database.')}
            </AlertDescription>
          </Alert>
        ) : (
          <div className="space-y-4">
            <div className="text-sm">
              <span className="text-muted-foreground">{t('User')}：</span>
              <span className="font-medium">
                {user.display_name || user.username}
              </span>
              {user.display_name && (
                <span className="text-muted-foreground ml-1">
                  @{user.username}
                </span>
              )}
              <span className="ml-3 text-muted-foreground">
                {t('Current role')}：{t(ROLE_META[user.role]?.labelKey ?? 'User')}
              </span>
            </div>

            <div>
              <label className="text-sm font-medium">{t('Target Role')}</label>
              <Select value={targetRole} onValueChange={setTargetRole}>
                <SelectTrigger className="mt-1.5">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {roleOptions.map((r) => (
                    <SelectItem key={r} value={String(r)}>
                      <span className="flex items-center gap-2">
                        <span>{t(ROLE_META[r].labelKey)}</span>
                        <span className="text-muted-foreground text-xs">
                          — {t(ROLE_META[r].descKey)}
                        </span>
                      </span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <Alert>
              <AlertTitle>{t('Role change notes')}</AlertTitle>
              <AlertDescription>
                <ul className="list-disc pl-4 text-xs space-y-1">
                  <li>{t('role_note_permission')}</li>
                  <li>{t('role_note_staff')}</li>
                  <li>{t('role_note_immediate')}</li>
                </ul>
              </AlertDescription>
            </Alert>
          </div>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          {!isRootUser && (
            <Button
              onClick={handleConfirm}
              disabled={loading || targetRole === String(user.role)}
            >
              {loading ? t('Saving...') : t('Save Role')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
