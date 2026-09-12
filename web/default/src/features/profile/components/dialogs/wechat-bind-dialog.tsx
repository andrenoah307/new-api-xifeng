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
import { Loader2, QrCode } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useStatus } from '@/hooks/use-status'

import { bindWeChat } from '../../api'

// ============================================================================
// WeChat Bind Dialog Component
// ============================================================================

interface WeChatBindDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: () => void
}

export function WeChatBindDialog({
  open,
  onOpenChange,
  onSuccess,
}: WeChatBindDialogProps) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const [loading, setLoading] = useState(false)
  const [code, setCode] = useState('')

  // 与注册页同源的兼容链（sign-up-form.tsx:125-137）：后端不同版本的
  // /api/status 用过多个字段名承载公众号二维码地址。
  const qrCodeUrl = useMemo(() => {
    return (
      status?.wechat_qrcode ||
      status?.wechat_qr_code ||
      status?.wechat_qrcode_image_url ||
      status?.wechat_qr_code_image_url ||
      status?.wechat_account_qrcode_image_url ||
      status?.WeChatAccountQRCodeImageURL ||
      status?.data?.wechat_qrcode ||
      status?.data?.WeChatAccountQRCodeImageURL ||
      ''
    )
  }, [status])

  const handleOpenChange = (next: boolean) => {
    if (loading) return
    onOpenChange(next)
    if (!next) {
      setCode('')
    }
  }

  const handleBind = async () => {
    const verificationCode = code.trim()
    if (!verificationCode) {
      toast.error(t('Please enter the verification code'))
      return
    }

    try {
      setLoading(true)
      const response = await bindWeChat(verificationCode)

      if (response.success) {
        toast.success(t('WeChat account bound successfully!'))
        handleOpenChange(false)
        onSuccess()
      } else {
        toast.error(response.message || t('Failed to bind WeChat account'))
      }
    } catch (_error) {
      toast.error(t('Failed to bind WeChat account'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={handleOpenChange}
      title={t('Bind WeChat Account')}
      description={t(
        'Scan the QR code to follow the official account and reply with “验证码” to receive your verification code.'
      )}
      contentClassName='sm:max-w-md'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => handleOpenChange(false)}
            disabled={loading}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='button'
            onClick={handleBind}
            disabled={loading || !code.trim()}
          >
            {loading && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {loading ? t('Binding...') : t('Bind WeChat Account')}
          </Button>
        </>
      }
    >
      <div className='space-y-4 py-4'>
        <div className='flex flex-col items-center justify-center rounded-lg border border-dashed p-4'>
          {qrCodeUrl ? (
            <img
              src={qrCodeUrl}
              alt={t('WeChat login QR code')}
              className='h-40 w-40 object-contain'
            />
          ) : (
            <>
              <QrCode className='text-muted-foreground mb-3 h-16 w-16' />
              <p className='text-muted-foreground text-sm'>
                {t('This feature requires server-side WeChat configuration')}
              </p>
            </>
          )}
        </div>

        <div className='space-y-2'>
          <Label htmlFor='wechat-bind-code'>{t('Verification Code')}</Label>
          <Input
            id='wechat-bind-code'
            value={code}
            onChange={(e) => setCode(e.target.value)}
            placeholder={t('Enter code')}
            disabled={loading}
            autoComplete='one-time-code'
          />
        </div>
      </div>
    </Dialog>
  )
}
