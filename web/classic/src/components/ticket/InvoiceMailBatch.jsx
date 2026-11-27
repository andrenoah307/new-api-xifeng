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
import React, { useContext, useEffect, useRef, useState } from 'react';
import { Button, Modal } from '@douyinfe/semi-ui';
import { Mail, RefreshCw, Send, Square, Upload } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API } from '../../helpers';
import { UserContext } from '../../context/User';

export default function InvoiceMailBatch() {
  const { t } = useTranslation();
  const [userState] = useContext(UserContext);
  const [open, setOpen] = useState(false);
  const [batch, setBatch] = useState(null);
  const [recent, setRecent] = useState([]);
  const [busy, setBusy] = useState(false);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState('');
  const [confirmation, setConfirmation] = useState(null);
  const stop = useRef(false);
  useEffect(
    () => () => {
      stop.current = true;
    },
    [],
  );
  const role = userState?.user?.role;
  const load = async (id) => {
    const response = await API.get(`/api/ticket/invoice-mail/${id}`, {
      disableDuplicate: true,
    });
    if (!response.data.success) throw new Error(response.data.message);
    setBatch(response.data.data);
  };
  const refresh = async (id) => {
    setBusy(true);
    setError('');
    try {
      await load(id);
    } catch {
      setError(t('Invoice batch unavailable'));
    } finally {
      setBusy(false);
    }
  };
  const list = async () => {
    const response = await API.get('/api/ticket/invoice-mail', {
      disableDuplicate: true,
    });
    if (!response.data.success) throw new Error(response.data.message);
    setRecent(response.data.data);
  };
  const upload = async (file) => {
    setBusy(true);
    setError('');
    try {
      if (
        !file.name.toLowerCase().endsWith('.zip') ||
        file.size > 25 * 1024 * 1024
      )
        throw new Error('archive');
      const form = new FormData();
      form.append('file', file);
      const response = await API.post('/api/ticket/invoice-mail', form, {
        timeout: 120000,
      });
      if (!response.data.success) throw new Error(response.data.message);
      setBatch(response.data.data);
      await list();
    } catch {
      setError(t('Invalid invoice ZIP archive'));
    } finally {
      setBusy(false);
    }
  };
  const send = async () => {
    if (!batch || !confirmation || busy) return;
    const request = confirmation;
    setConfirmation(null);
    setBusy(true);
    setSending(true);
    setError('');
    stop.current = false;
    const items = batch.items.filter((item) =>
      request.ticket
        ? item.ticket_id === request.ticket
        : ['ready', 'failed'].includes(item.state),
    );
    try {
      for (const item of items) {
        if (stop.current) break;
        setBatch((previous) => ({
          ...previous,
          items: previous.items.map((row) =>
            row.ticket_id === item.ticket_id
              ? { ...row, state: 'sending' }
              : row,
          ),
        }));
        const response = await API.post(
          `/api/ticket/invoice-mail/${batch.id}/${item.ticket_id}/send`,
          { review: request.review || '' },
          { timeout: 60000 },
        );
        if (!response.data.success) throw new Error(response.data.message);
        setBatch((previous) => ({
          ...previous,
          items: previous.items.map((row) =>
            row.ticket_id === item.ticket_id
              ? { ...row, state: 'sent', reason: '' }
              : row,
          ),
        }));
      }
    } catch {
      setError(t('Invoice delivery failed; refresh the batch before retrying'));
    } finally {
      try {
        await load(batch.id);
      } catch {
        setError(t('Invoice batch unavailable'));
      }
      setBusy(false);
      setSending(false);
    }
  };
  if (role !== 3 && role !== 100) return null;
  const ready =
    batch?.items.filter((item) => ['ready', 'failed'].includes(item.state))
      .length || 0;
  const labels = {
    ready: t('Ready to send'),
    failed: t('Failed'),
    sent: t('Sent'),
    sending: t('Sending'),
    unknown: t('Delivery needs review'),
    blocked: t('Blocked'),
  };
  return (
    <>
      <Button
        icon={<Upload size={16} />}
        onClick={() => {
          setOpen(true);
          void list().catch(() => setError(t('Invoice batch unavailable')));
        }}
      >
        {t('Batch invoice upload')}
      </Button>
      <Modal
        centered
        visible={open}
        title={t('Batch invoice upload')}
        footer={null}
        width='min(960px, 96vw)'
        maskClosable={!busy}
        closable={!busy}
        closeOnEsc={!busy}
        onCancel={() => {
          if (!busy) setOpen(false);
        }}
        bodyStyle={{ maxHeight: '80dvh', overflowY: 'auto' }}
      >
        <div className='space-y-4'>
          <div className='grid grid-cols-[minmax(0,1fr)_auto] items-end gap-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]'>
            <label className='col-span-2 min-w-0 space-y-2 sm:col-span-1'>
              <span>{t('Invoice ZIP (up to 25 MB)')}</span>
              <input
                className='w-full text-sm'
                type='file'
                accept='.zip,application/zip'
                disabled={busy}
                onChange={(event) => {
                  const file = event.target.files?.[0];
                  if (file) void upload(file);
                  event.target.value = '';
                }}
              />
            </label>
            <label className='min-w-0 space-y-2'>
              <span>{t('Recent invoice batches')}</span>
              <select
                className='h-9 w-full rounded border bg-transparent px-2 text-sm'
                value={batch?.id || ''}
                disabled={busy}
                onChange={(event) => {
                  if (event.target.value) void refresh(event.target.value);
                }}
              >
                <option value=''>{t('Select')}</option>
                {recent.map((row) => (
                  <option key={row.id} value={row.id}>
                    {new Date(row.created_at * 1000).toLocaleString()} ·{' '}
                    {row.id.slice(0, 8)}
                  </option>
                ))}
              </select>
            </label>
            <Button
              icon={<RefreshCw size={16} />}
              title={t('Refresh')}
              aria-label={t('Refresh')}
              disabled={busy || !batch}
              onClick={() => void refresh(batch.id)}
            />
          </div>
          {error && (
            <p role='alert' className='text-sm text-red-600'>
              {error}
            </p>
          )}
          {busy && !sending && <p role='status'>{t('Loading...')}</p>}
          {batch && (
            <>
              <div
                className='flex flex-wrap justify-between gap-2 text-sm'
                aria-live='polite'
              >
                <span>
                  {t('Invoices')}: {batch.items.length} · {t('Attachments')}:{' '}
                  {batch.items.reduce(
                    (sum, item) => sum + item.files.length,
                    0,
                  )}{' '}
                  · {t('Sent')}:{' '}
                  {batch.items.filter((item) => item.state === 'sent').length}
                </span>
                <span>
                  {t('Expires at')}:{' '}
                  {new Date(batch.expires_at * 1000).toLocaleString()}
                </span>
              </div>
              <div className='max-h-[40dvh] overflow-auto border-y'>
                <div className='divide-y sm:hidden'>
                  {batch.items.map((item) => (
                    <div
                      key={item.ticket_id}
                      className='space-y-2 py-3 text-sm'
                    >
                      <div className='flex flex-wrap justify-between gap-2 font-medium'>
                        <span>#{item.ticket_id}</span>
                        <span>{labels[item.state] || item.state}</span>
                      </div>
                      <p className='break-words'>{item.subject}</p>
                      <p className='break-all'>{item.recipient || '-'}</p>
                      <details>
                        <summary>
                          {t('Attachments')}: {item.files.length}
                        </summary>
                        {item.files.map((name) => (
                          <div key={name} className='break-all'>
                            {name}
                          </div>
                        ))}
                      </details>
                      {item.reason && item.state !== 'sent' && (
                        <p className='text-red-600'>{t(item.reason)}</p>
                      )}
                      {item.state === 'unknown' && (
                        <div className='flex flex-wrap gap-2'>
                          <Button
                            size='small'
                            disabled={busy}
                            icon={<Mail size={14} />}
                            onClick={() =>
                              setConfirmation({
                                ticket: item.ticket_id,
                                review: 'delivered',
                              })
                            }
                          >
                            {t('Confirm delivered')}
                          </Button>
                          <Button
                            size='small'
                            disabled={busy}
                            icon={<Send size={14} />}
                            onClick={() =>
                              setConfirmation({
                                ticket: item.ticket_id,
                                review: 'not_delivered',
                              })
                            }
                          >
                            {t('Confirm not delivered and retry')}
                          </Button>
                        </div>
                      )}
                    </div>
                  ))}
                </div>
                <table className='hidden w-full text-left text-sm sm:table'>
                  <thead>
                    <tr className='border-b'>
                      <th className='p-2'>{t('Ticket')}</th>
                      <th className='p-2'>{t('Recipient')}</th>
                      <th className='p-2'>{t('Attachments')}</th>
                      <th className='p-2'>{t('Status')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {batch.items.map((item) => (
                      <tr key={item.ticket_id} className='border-b align-top'>
                        <td className='max-w-48 break-words p-2'>
                          #{item.ticket_id}
                          <div>{item.subject}</div>
                        </td>
                        <td className='max-w-56 break-all p-2'>
                          {item.recipient || '-'}
                        </td>
                        <td className='p-2'>
                          <details>
                            <summary className='cursor-pointer'>
                              {item.files.length}
                            </summary>
                            {item.files.map((name, index) => (
                              <div key={index} className='max-w-48 break-all'>
                                {name}
                              </div>
                            ))}
                          </details>
                        </td>
                        <td className='max-w-64 p-2'>
                          {labels[item.state] || item.state}
                          {item.reason && item.state !== 'sent' && (
                            <div className='text-red-600'>{t(item.reason)}</div>
                          )}
                          {item.state === 'unknown' && (
                            <div className='mt-2 flex flex-wrap gap-2'>
                              <Button
                                size='small'
                                disabled={busy}
                                icon={<Mail size={14} />}
                                onClick={() =>
                                  setConfirmation({
                                    ticket: item.ticket_id,
                                    review: 'delivered',
                                  })
                                }
                              >
                                {t('Confirm delivered')}
                              </Button>
                              <Button
                                size='small'
                                disabled={busy}
                                icon={<Send size={14} />}
                                onClick={() =>
                                  setConfirmation({
                                    ticket: item.ticket_id,
                                    review: 'not_delivered',
                                  })
                                }
                              >
                                {t('Confirm not delivered and retry')}
                              </Button>
                            </div>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <dl className='space-y-2 text-sm'>
                <dt className='font-medium'>{t('Reply after delivery')}</dt>
                <dd className='whitespace-pre-wrap break-words'>
                  {batch.reply}
                </dd>
                <dt className='font-medium'>
                  {t('Ticket status after delivery')}
                </dt>
                <dd>
                  {batch.status === 3 ? t('Resolved') : t('Processing')} ·{' '}
                  {t('Issued')}
                </dd>
              </dl>
              {batch.ignored?.length > 0 && (
                <details>
                  <summary>
                    {t('Ignored files')}: {batch.ignored.length}
                  </summary>
                  <ul className='max-h-32 overflow-auto text-sm'>
                    {batch.ignored.map((name, index) => (
                      <li key={index} className='break-all'>
                        {name}
                      </li>
                    ))}
                  </ul>
                </details>
              )}
              {batch.cleaned_at > 0 && (
                <p role='status' className='text-sm'>
                  {t('Temporary invoice files deleted')}
                </p>
              )}
              <div className='flex flex-wrap justify-end gap-2'>
                {sending ? (
                  <Button
                    icon={<Square size={16} />}
                    onClick={() => {
                      stop.current = true;
                    }}
                  >
                    {t('Stop')}
                  </Button>
                ) : (
                  <Button
                    theme='solid'
                    disabled={busy || ready === 0}
                    icon={<Send size={16} />}
                    onClick={() => setConfirmation({})}
                  >
                    {t('Send invoices')} ({ready})
                  </Button>
                )}
              </div>
            </>
          )}
        </div>
      </Modal>
      <Modal
        centered
        visible={confirmation !== null}
        title={t('Confirm invoice delivery')}
        onCancel={() => setConfirmation(null)}
        onOk={() => void send()}
        okText={t('Confirm')}
        cancelText={t('Cancel')}
      >
        {confirmation?.review
          ? t(
              'Verify the SMTP delivery record before confirming. This action updates the invoice and ticket.',
            )
          : t(
              'Send attachments to the previewed recipients and update these tickets?',
            )}
      </Modal>
    </>
  );
}
