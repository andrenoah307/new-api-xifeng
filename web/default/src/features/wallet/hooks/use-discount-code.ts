import { useState, useCallback } from 'react'
import i18next from 'i18next'
import { toast } from 'sonner'
import { validateDiscountCode } from '@/features/discount-codes/api'

export interface DiscountInfo {
  discount_rate: number
  code: string
}

export function useDiscountCode() {
  const [discountCode, setDiscountCode] = useState('')
  const [discountInfo, setDiscountInfo] = useState<DiscountInfo | null>(null)
  const [isValidating, setIsValidating] = useState(false)

  const validateCode = useCallback(async () => {
    if (!discountCode.trim()) return

    setIsValidating(true)
    try {
      const result = await validateDiscountCode(discountCode.trim())
      if (result.success && result.data) {
        setDiscountInfo(result.data)
        const off = 100 - result.data.discount_rate
        toast.success(
          i18next.t('Discount code valid: {{off}}% off (pay {{rate}}%)', {
            off,
            rate: result.data.discount_rate,
          })
        )
      } else {
        setDiscountInfo(null)
        toast.error(
          result.message || i18next.t('Invalid discount code')
        )
      }
    } catch {
      setDiscountInfo(null)
      toast.error(i18next.t('Failed to validate discount code'))
    } finally {
      setIsValidating(false)
    }
  }, [discountCode])

  const clearDiscountCode = useCallback(() => {
    setDiscountCode('')
    setDiscountInfo(null)
  }, [])

  return {
    discountCode,
    setDiscountCode,
    discountInfo,
    isValidating,
    validateDiscountCode: validateCode,
    clearDiscountCode,
  }
}
