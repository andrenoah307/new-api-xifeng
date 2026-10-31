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
import { ChevronLeft, ChevronRight, RefreshCw } from 'lucide-react';
import React, { useEffect, useState } from 'react';
import { Modal, Button } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API } from '../../helpers';

export default function CostConfirmation(props) {
  const { t } = useTranslation();
  const [preview, setPreview] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [reload, setReload] = useState(0);
  const [filter, setFilter] = useState('');
  const [page, setPage] = useState(1);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setPreview(null);
    setError('');
    API.get('/api/channel-finance/cost-preview', {
      params: { channel_id: props.request.channel_id },
      signal: controller.signal,
      disableDuplicate: true,
      skipErrorHandler: true,
    })
      .then((result) => {
        if (controller.signal.aborted) return;
        if (!result.data.success) throw new Error(t('Failed to load report'));
        setPreview(result.data.data);
      })
      .catch((failure) => {
        if (!controller.signal.aborted) setError(failure.message);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [props.request.channel_id, props.refresh, reload, t]);
  const rows =
    preview?.channels.filter((row) =>
      `${row.channel_id} ${row.name}`
        .toLowerCase()
        .includes(filter.toLowerCase()),
    ) ?? [];
  const pages = Math.max(1, Math.ceil(rows.length / 20));
  const currentPage = Math.min(page, pages);
  return (
    <Modal
      visible
      title={t('Confirm channel cost estimates')}
      style={{ width: 'min(680px, 94vw)' }}
      bodyStyle={{ maxHeight: '65vh', overflowY: 'auto' }}
      onCancel={() => {
        if (!props.busy) props.onClose();
      }}
      closeOnEsc={!props.busy}
      maskClosable={!props.busy}
      footer={
        <div className='flex flex-wrap justify-end gap-2'>
          <Button
            title={t('Refresh')}
            aria-label={t('Refresh')}
            disabled={props.busy || loading}
            onClick={() => setReload((value) => value + 1)}
          >
            <RefreshCw size={16} />
          </Button>
          <Button disabled={props.busy} onClick={props.onClose}>
            {t('Cancel')}
          </Button>
          <Button
            theme='solid'
            loading={props.busy}
            disabled={
              props.busy || loading || !!error || !preview?.channels.length
            }
            onClick={() => props.onConfirm(preview.fingerprint)}
          >
            {t('Confirm and generate')}
          </Button>
        </div>
      }
    >
      <p>
        {t(
          'Current channel cost settings apply to the entire report period. Channels without usable cost settings are skipped.',
        )}
      </p>
      <p className='my-3'>
        {new Date(props.request.start * 1000).toLocaleString()} -{' '}
        {new Date(props.request.end * 1000).toLocaleString()}
      </p>
      <input
        className='w-full rounded border p-2'
        aria-label={t('Filter channels')}
        placeholder={t('Filter channels')}
        value={filter}
        onChange={(event) => {
          setFilter(event.target.value);
          setPage(1);
        }}
      />
      {loading && <p role='status'>{t('Loading')}</p>}
      {(error || props.error) && (
        <p role='alert' className='text-red-600'>
          {error || props.error}
        </p>
      )}
      <div className='overflow-x-auto'>
        <table className='w-full text-sm'>
          <thead>
            <tr>
              <th className='p-2 text-left'>{t('Channel')}</th>
              <th className='p-2 text-right'>{t('Cost ratio')}</th>
            </tr>
          </thead>
          <tbody>
            {rows.slice((currentPage - 1) * 20, currentPage * 20).map((row) => (
              <tr key={row.channel_id} className='border-t'>
                <td className='max-w-64 break-words p-2'>
                  #{row.channel_id} {row.name}
                </td>
                <td className='p-2 text-right'>
                  {row.ratio}
                  {row.peak && (
                    <div>
                      {row.peak.start}-{row.peak.end} ({row.peak.timezone}) ×{' '}
                      {row.peak.ratio}
                    </div>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {preview && !preview.channels.length && (
        <p>{t('No channels have usable cost settings.')}</p>
      )}
      <div className='my-3 flex flex-wrap items-center justify-between gap-2'>
        <span>
          {t('Total')}: {preview?.channels.length ?? 0} ·{' '}
          {t('Skipped channels: {{count}}', { count: preview?.skipped ?? 0 })}
        </span>
        <div className='flex items-center gap-2'>
          <Button
            title={t('Previous')}
            aria-label={t('Previous')}
            disabled={currentPage <= 1}
            onClick={() => setPage(currentPage - 1)}
          >
            <ChevronLeft size={16} />
          </Button>
          <span>
            {currentPage}/{pages}
          </span>
          <Button
            title={t('Next')}
            aria-label={t('Next')}
            disabled={currentPage >= pages}
            onClick={() => setPage(currentPage + 1)}
          >
            <ChevronRight size={16} />
          </Button>
        </div>
      </div>
    </Modal>
  );
}
