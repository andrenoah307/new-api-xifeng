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

import React, { useContext, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Checkbox,
  DatePicker,
  Input,
  Modal,
  Select,
  Space,
  Table,
  Typography,
} from '@douyinfe/semi-ui';
import { IconDownload, IconSetting } from '@douyinfe/semi-icons';
import { API, showError, showSuccess, timestamp2string } from '../../helpers';
import { UserContext } from '../../context/User';

export default function Finance() {
  const { t } = useTranslation();
  const [state] = useContext(UserContext);
  const user = state.user;
  const canRead =
    user?.role === 100 ||
    user?.permissions?.admin_permissions?.finance?.read === true;
  const [policy, setPolicy] = useState(null);
  const [requestedView, setView] = useState('invoices');
  const [keyword, setKeyword] = useState('');
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState('');
  const [dates, setDates] = useState([]);
  const [page, setPage] = useState(1);
  const [result, setResult] = useState({ items: [], total: 0 });
  const [loading, setLoading] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [draft, setDraft] = useState(null);
  const [choices, setChoices] = useState([]);
  const [saving, setSaving] = useState(false);
  const view =
    (requestedView === 'invoices' && policy?.invoice_statuses?.length) ||
    !policy?.topup_statuses?.length
      ? 'invoices'
      : 'topups';
  useEffect(() => {
    if (!canRead) return;
    let active = true;
    API.get('/api/finance/policy')
      .then(({ data }) => {
        if (!data.success) throw new Error(data.message);
        if (active) setPolicy(data.data);
      })
      .catch((error) => {
        if (active) showError(error.message);
      });
    return () => {
      active = false;
    };
  }, [canRead, user?.id]);
  const params = useMemo(
    () => ({
      view,
      keyword: search || undefined,
      status: view === 'topups' ? status || undefined : undefined,
      invoice_status: view === 'invoices' ? status || undefined : undefined,
      start_time: dates[0]
        ? Math.floor(new Date(dates[0]).getTime() / 1000)
        : undefined,
      end_time: dates[1]
        ? Math.floor(new Date(dates[1]).getTime() / 1000)
        : undefined,
    }),
    [view, search, status, dates],
  );
  useEffect(() => {
    if (!canRead || !policy) return;
    const controller = new AbortController();
    setLoading(true);
    setResult({ items: [], total: 0 });
    API.get('/api/finance/records', {
      params: { ...params, p: page, page_size: 20 },
      signal: controller.signal,
    })
      .then(({ data }) => {
        if (!data.success) throw new Error(data.message);
        if (!controller.signal.aborted) setResult(data.data);
      })
      .catch((error) => {
        if (!controller.signal.aborted) showError(error.message);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [params, page, policy, canRead, user?.id]);
  const invoiceLabels = {
    1: t('Pending Issuance'),
    2: t('Issued'),
    3: t('Rejected'),
  };
  const labels = {
    success: t('Success'),
    pending: t('Pending'),
    failed: t('Failed'),
    expired: t('Expired'),
  };
  const columns = [
    {
      title: t('User'),
      dataIndex: 'username',
      render: (value, item) => `${value || ''} (#${item.user_id})`,
    },
    { title: t('Invoice title'), dataIndex: 'company_name' },
    {
      title: view === 'invoices' ? t('Invoice amount') : t('Payment Amount'),
      dataIndex: 'money',
      render: (value) => Number(value).toFixed(2),
    },
    {
      title: t('Status'),
      render: (_, item) =>
        view === 'invoices'
          ? invoiceLabels[item.invoice_status]
          : labels[item.status],
    },
    {
      title: t('Created Time'),
      dataIndex: 'created_time',
      render: timestamp2string,
    },
    {
      title: t('Ticket ID'),
      dataIndex: 'ticket_id',
      render: (value) => value || '-',
    },
  ];
  const exportRecords = async () => {
    setExporting(true);
    try {
      const response = await API.get('/api/finance/export', {
        params,
        responseType: 'blob',
      });
      if (!String(response.headers['content-type']).includes('text/csv'))
        throw new Error(JSON.parse(await response.data.text()).message);
      const url = URL.createObjectURL(response.data);
      const link = document.createElement('a');
      link.href = url;
      link.download = `finance-${view}.csv`;
      link.click();
      URL.revokeObjectURL(url);
    } catch (error) {
      showError(error.message);
    } finally {
      setExporting(false);
    }
  };
  const openPolicy = async () => {
    try {
      const { data } = await API.get('/api/finance/policy/choices');
      if (!data.success) throw new Error(data.message);
      setChoices(data.data || []);
      setDraft({
        invoice_statuses: policy.invoice_statuses || [],
        topup_statuses: policy.topup_statuses || [],
        payment_methods: policy.payment_methods || [],
        sources: policy.sources || [],
      });
    } catch (error) {
      showError(error.message);
    }
  };
  const savePolicy = async () => {
    setSaving(true);
    try {
      const { data } = await API.put('/api/finance/policy', draft);
      if (!data.success) throw new Error(data.message);
      setPolicy(draft);
      setDraft(null);
      setPage(1);
      setStatus('');
      showSuccess(t('Saved successfully'));
    } catch (error) {
      showError(error.message);
    } finally {
      setSaving(false);
    }
  };
  if (!canRead) return null;
  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap justify-between gap-3'>
        <Space wrap>
          {user?.role >= 10 && (
            <Button
              icon={<IconSetting />}
              disabled={!policy}
              onClick={openPolicy}
            >
              {t('Financial record visibility')}
            </Button>
          )}
          {(user?.role === 100 ||
            user?.permissions?.admin_permissions?.finance?.export === true) && (
            <Button
              icon={<IconDownload />}
              loading={exporting}
              disabled={loading || result.total === 0}
              onClick={exportRecords}
            >
              {t('Export filtered records')}
            </Button>
          )}
        </Space>
      </div>
      <Space wrap>
        <Select
          value={view}
          onChange={(value) => {
            setView(value);
            setStatus('');
            setPage(1);
          }}
          optionList={[
            ...((policy?.invoice_statuses || []).length
              ? [{ value: 'invoices', label: t('Invoice applications') }]
              : []),
            ...((policy?.topup_statuses || []).length
              ? [{ value: 'topups', label: t('All-user top-up records') }]
              : []),
          ]}
        />
        <Input
          value={keyword}
          onChange={setKeyword}
          placeholder={t('Search')}
          onEnterPress={() => {
            setSearch(keyword);
            setPage(1);
          }}
        />
        <Button
          onClick={() => {
            setSearch(keyword);
            setPage(1);
          }}
        >
          {t('Search')}
        </Button>
        <Select
          value={status}
          onChange={(value) => {
            setStatus(value);
            setPage(1);
          }}
          optionList={[
            { value: '', label: t('All') },
            ...(view === 'invoices'
              ? policy?.invoice_statuses || []
              : policy?.topup_statuses || []
            ).map((value) => ({
              value: String(value),
              label: view === 'invoices' ? invoiceLabels[value] : labels[value],
            })),
          ]}
        />
        <DatePicker
          type='dateTimeRange'
          value={dates}
          onChange={(value) => {
            setDates(value || []);
            setPage(1);
          }}
        />
      </Space>
      <Table
        rowKey='id'
        columns={columns}
        dataSource={result.items}
        loading={loading}
        scroll={{ x: 900 }}
        pagination={{
          currentPage: page,
          pageSize: 20,
          total: result.total,
          onPageChange: setPage,
        }}
        expandedRowRender={(item) => (
          <>
            {item.incomplete_orders && (
              <Typography.Text type='warning'>
                {t('Some linked top-ups are unavailable')}
              </Typography.Text>
            )}
            <Table
              rowKey='id'
              pagination={false}
              dataSource={item.topups}
              columns={[
                { title: t('Order Number'), dataIndex: 'trade_no' },
                {
                  title: t('Payment Amount'),
                  dataIndex: 'money',
                  render: (value) => Number(value).toFixed(2),
                },
                { title: t('Payment Method'), dataIndex: 'payment_method' },
                { title: t('Source'), dataIndex: 'source' },
                {
                  title: t('Status'),
                  dataIndex: 'status',
                  render: (value) => labels[value],
                },
              ]}
            />
          </>
        )}
      />
      <Modal
        title={t('Financial record visibility')}
        visible={user?.role >= 10 && !!draft}
        centered
        bodyStyle={{ maxHeight: '70vh', overflowY: 'auto' }}
        getPopupContainer={() => document.body}
        confirmLoading={saving}
        onCancel={() => {
          if (!saving) setDraft(null);
        }}
        onOk={savePolicy}
      >
        {draft && (
          <div className='space-y-4'>
            <div>
              <Typography.Text strong>
                {t('Invoice applications')}
              </Typography.Text>
              <Checkbox.Group
                value={draft.invoice_statuses}
                onChange={(value) =>
                  setDraft((prev) => ({ ...prev, invoice_statuses: value }))
                }
                options={[1, 2, 3].map((value) => ({
                  value,
                  label: invoiceLabels[value],
                }))}
              />
            </div>
            <div>
              <Typography.Text strong>
                {t('All-user top-up records')}
              </Typography.Text>
              <Checkbox.Group
                value={draft.topup_statuses}
                onChange={(value) =>
                  setDraft((prev) => ({ ...prev, topup_statuses: value }))
                }
                options={Object.keys(labels).map((value) => ({
                  value,
                  label: labels[value],
                }))}
              />
            </div>
            <div>
              <Typography.Text strong>
                {t('Allowed payment methods')}
              </Typography.Text>
              <Select
                multiple
                className='w-full'
                placeholder={t('All payment methods')}
                value={draft.payment_methods}
                onChange={(value) =>
                  setDraft((prev) => ({ ...prev, payment_methods: value }))
                }
                optionList={[
                  ...new Set([
                    ...choices.map((item) => item.payment_method),
                    ...draft.payment_methods,
                  ]),
                ].map((value) => ({ value, label: value || t('Unspecified') }))}
              />
            </div>
            <div>
              <Typography.Text strong>
                {t('Allowed top-up sources')}
              </Typography.Text>
              <Select
                multiple
                className='w-full'
                placeholder={t('All top-up sources')}
                value={draft.sources}
                onChange={(value) =>
                  setDraft((prev) => ({ ...prev, sources: value }))
                }
                optionList={[
                  ...new Set([
                    ...choices.map((item) => item.source),
                    ...draft.sources,
                  ]),
                ].map((value) => ({ value, label: value || t('Unspecified') }))}
              />
            </div>
            <Typography.Paragraph type='tertiary'>
              {t(
                'An invoice is visible only when all linked top-ups match the allowed payment methods and sources.',
              )}
            </Typography.Paragraph>
          </div>
        )}
      </Modal>
    </div>
  );
}
