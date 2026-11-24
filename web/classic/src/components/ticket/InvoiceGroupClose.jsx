/*
Copyright (C) 2025 QuantumNous

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
import React, { useEffect, useRef, useState } from 'react';
import { Button, Checkbox, Modal } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError } from '../../helpers';
import {
  previewInvoiceGroupClosures,
  closeInvoiceGroupTicket,
} from '../../../../shared/invoice-group-close';

export default function InvoiceGroupClose({ disabled }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [convert, setConvert] = useState(false);
  const [busy, setBusy] = useState(false);
  const [items, setItems] = useState(null);
  const active = useRef(true);
  const running = useRef(false);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);
  const preview = async () => {
    if (running.current) return;
    running.current = true;
    setBusy(true);
    setItems(null);
    try {
      const rows = await previewInvoiceGroupClosures(API.post, convert);
      if (active.current) setItems(rows);
    } catch (error) {
      if (active.current) showError(error.message);
    } finally {
      running.current = false;
      if (active.current) setBusy(false);
    }
  };
  const execute = async () => {
    if (running.current || !items) return;
    running.current = true;
    setBusy(true);
    const results = [...items];
    try {
      for (let i = 0; i < results.length && active.current; i++) {
        if (results[i].applied) continue;
        try {
          results[i] = await closeInvoiceGroupTicket(
            API.post,
            results[i],
            convert,
          );
        } catch (error) {
          results[i] = { ...results[i], error: error.message };
        }
        if (active.current) setItems([...results]);
      }
    } finally {
      running.current = false;
      if (active.current) setBusy(false);
    }
  };
  return (
    <>
      <Button
        style={{ height: 'auto', whiteSpace: 'normal', maxWidth: '100%' }}
        disabled={disabled}
        onClick={() => {
          setItems(null);
          setOpen(true);
        }}
      >
        {t('Close pending invoice tickets in selected groups')}
      </Button>
      <Modal
        visible={open}
        centered
        width={900}
        style={{ maxWidth: 'calc(100vw - 32px)' }}
        bodyStyle={{ overflowY: 'auto', maxHeight: '70vh' }}
        title={t('Close pending invoice tickets in selected groups')}
        closable={!busy}
        maskClosable={!busy}
        onCancel={() => {
          if (!busy) setOpen(false);
        }}
        footer={null}
      >
        <div className='space-y-4'>
          <p>
            {t(
              'Close all listed tickets. Issued invoices remain unchanged. Tax conversion does not block ticket closure.',
            )}
          </p>
          <Checkbox
            checked={convert}
            disabled={busy}
            onChange={(e) => {
              setConvert(e.target.checked);
              setItems(null);
            }}
          >
            {t(
              'Also convert prepaid order tax to balance and remove invoice eligibility',
            )}
          </Checkbox>
          <div>
            <Button disabled={busy} onClick={preview}>
              {t('Preview')}
            </Button>
          </div>
          {items && (
            <div className='overflow-x-auto'>
              <table className='w-full min-w-[640px] text-left text-sm [&_th]:px-2 [&_th]:py-2 [&_td]:px-2 [&_td]:py-2'>
                <thead>
                  <tr>
                    <th>{t('Ticket ID')}</th>
                    <th>{t('User ID')}</th>
                    <th>{t('Invoice title')}</th>
                    <th>{t('Invoice amount')}</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((item) => (
                    <tr key={item.ticket_id} className='border-t'>
                      <td>{item.ticket_id}</td>
                      <td>
                        {item.user_id} ({item.username})
                      </td>
                      <td className='max-w-64 break-words'>
                        {item.invoice_title || '-'}
                      </td>
                      <td>{(item.amount ?? 0).toFixed(2)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {items.length === 0 && <p>{t('No data')}</p>}
            </div>
          )}
          {items?.some((item) => item.applied || item.error) && (
            <div role='status' className='space-y-2 text-sm'>
              <p>
                {t('Closed')}: {items.filter((item) => item.applied).length} /{' '}
                {items.length}
              </p>
              {items
                .filter((item) => item.error || item.warning)
                .map((item) => (
                  <p key={item.ticket_id}>
                    #{item.ticket_id}: {t(item.error || item.warning || '')}
                  </p>
                ))}
            </div>
          )}
          <Button
            theme='solid'
            disabled={busy || !items?.some((item) => !item.applied)}
            onClick={execute}
          >
            {t('Close all listed tickets')}
          </Button>
        </div>
      </Modal>
    </>
  );
}
