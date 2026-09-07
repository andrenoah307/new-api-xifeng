/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Download } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { CopyButton } from '@/components/copy-button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { downloadTextFile } from '@/lib/download'

type GeneratedCodesDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  codes: string[]
  filename: string
  partial?: boolean
}

export function GeneratedCodesDialog(props: GeneratedCodesDialogProps) {
  const { t } = useTranslation()
  const contentToDownload = props.codes.join('\n')

  const handleDownload = () => {
    try {
      downloadTextFile(contentToDownload, `${props.filename}.txt`)
    } catch {
      toast.error(t('Download failed'))
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>{props.title}</DialogTitle>
          <DialogDescription>
            {t(
              'Save these codes now. They will not be shown again after you close this dialog.'
            )}
          </DialogDescription>
        </DialogHeader>

        {props.partial && (
          <Alert variant='warning'>
            <AlertDescription>
              {t(
                'Creation stopped partway. {{count}} code(s) were generated before the error.',
                { count: props.codes.length }
              )}
            </AlertDescription>
          </Alert>
        )}

        <div className='bg-muted/50 overflow-x-auto rounded-md border p-3 font-mono text-sm'>
          {props.codes.slice(0, 6).map((code) => (
            <div className='whitespace-nowrap' key={code}>
              {code}
            </div>
          ))}
        </div>
        {props.codes.length > 6 && (
          <p className='text-muted-foreground text-xs'>
            {t('Showing {{shown}} of {{count}}', {
              shown: 6,
              count: props.codes.length,
            })}
          </p>
        )}

        <DialogFooter>
          <Button variant='outline' onClick={handleDownload}>
            <Download className='mr-1.5 size-4' />
            {t('Download')}
          </Button>
          <CopyButton
            value={contentToDownload}
            variant='outline'
            size='default'
            successTooltip={t('Codes copied!')}
          >
            {t('Copy All Codes')}
          </CopyButton>
          <Button variant='default' onClick={() => props.onOpenChange(false)}>
            {t('Close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
