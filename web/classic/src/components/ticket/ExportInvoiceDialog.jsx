import React, { useState, useMemo, useCallback, useEffect } from 'react';
import {
  Button,
  Checkbox,
  DatePicker,
  Input,
  Modal,
  Select,
  Table,
  Typography,
} from '@douyinfe/semi-ui';
import { IconDownload } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../helpers';
import { timestamp2string } from '../../helpers';

const PAGE_SIZE = 20;

const TICKET_STATUS_OPTIONS = [
  { value: 0, label: '全部' },
  { value: 1, label: '待处理' },
  { value: 2, label: '处理中' },
  { value: 3, label: '已解决' },
  { value: 4, label: '已关闭' },
];

function csvField(value) {
  const s = String(value ?? '');
  if (s.includes(',') || s.includes('"') || s.includes('\n') || s.includes('\r')) {
    return '"' + s.replace(/"/g, '""') + '"';
  }
  return s;
}

function generateInvoiceCSV(items, serviceName) {
  const BOM = '﻿';
  const headers = ['工单ID', '电子邮箱', '数量', '单价', '金额合计', '公司信息', '应税服务名称'];
  const lines = [headers.map(csvField).join(',')];
  for (const item of items) {
    const companyInfo = item.tax_number
      ? `发票抬头\n${item.company_name}\n购方税号\n${item.tax_number}`
      : item.company_name;
    const row = [
      item.ticket_id,
      item.email,
      '',
      '',
      item.total_money.toFixed(2),
      companyInfo,
      serviceName,
    ];
    lines.push(row.map(csvField).join(','));
  }
  return BOM + lines.join('\r\n');
}

function downloadCSV(csv, filename) {
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

const statusLabel = (s, t) => {
  switch (s) {
    case 1: return t('待处理');
    case 2: return t('处理中');
    case 3: return t('已解决');
    case 4: return t('已关闭');
    default: return '-';
  }
};

export default function ExportInvoiceDialog({ visible, onClose }) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [items, setItems] = useState([]);
  const [statusFilter, setStatusFilter] = useState(0);
  const [keyword, setKeyword] = useState('');
  const [searchKeyword, setSearchKeyword] = useState('');
  const [serviceName, setServiceName] = useState('');
  const [dateRange, setDateRange] = useState([null, null]);
  const [selected, setSelected] = useState(new Map());

  const fetchData = useCallback(
    async (p, kw, status, dates) => {
      setLoading(true);
      try {
        const params = new URLSearchParams();
        params.set('p', String(p));
        params.set('page_size', String(PAGE_SIZE));
        if (kw) params.set('keyword', kw);
        if (status > 0) params.set('status', String(status));
        if (dates && dates[0]) params.set('start_time', String(Math.floor(dates[0] / 1000)));
        if (dates && dates[1]) params.set('end_time', String(Math.floor(dates[1] / 1000)));
        const res = await API.get(`/api/ticket/admin/invoice/export-list?${params}`);
        if (res.data?.success) {
          const data = res.data.data;
          setItems(data?.items ?? []);
          setTotal(data?.total ?? 0);
        } else {
          showError(res.data?.message || 'Failed to load');
        }
      } catch {
        showError(t('请求失败'));
      }
      setLoading(false);
    },
    [t],
  );

  useEffect(() => {
    if (visible) {
      setPage(1);
      setStatusFilter(0);
      setKeyword('');
      setSearchKeyword('');
      setSelected(new Map());
      setServiceName('');
      setDateRange([null, null]);
      fetchData(1, '', 0, [null, null]);
    }
  }, [visible]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleSearch = useCallback(() => {
    setSearchKeyword(keyword);
    setPage(1);
    setSelected(new Map());
    fetchData(1, keyword, statusFilter, dateRange);
  }, [keyword, statusFilter, dateRange, fetchData]);

  const handleStatusChange = useCallback(
    (v) => {
      setStatusFilter(v);
      setPage(1);
      setSelected(new Map());
      fetchData(1, searchKeyword, v, dateRange);
    },
    [searchKeyword, dateRange, fetchData],
  );

  const handleDateRangeChange = useCallback(
    (dates) => {
      const next = dates && dates.length === 2 ? dates : [null, null];
      setDateRange(next);
      setPage(1);
      setSelected(new Map());
      fetchData(1, searchKeyword, statusFilter, next);
    },
    [searchKeyword, statusFilter, fetchData],
  );

  const handlePageChange = useCallback(
    (p) => {
      setPage(p);
      fetchData(p, searchKeyword, statusFilter, dateRange);
    },
    [searchKeyword, statusFilter, dateRange, fetchData],
  );

  const toggleItem = useCallback((item) => {
    setSelected((prev) => {
      const next = new Map(prev);
      if (next.has(item.ticket_id)) {
        next.delete(item.ticket_id);
      } else {
        next.set(item.ticket_id, item);
      }
      return next;
    });
  }, []);

  const allOnPageSelected =
    items.length > 0 && items.every((i) => selected.has(i.ticket_id));

  const toggleAll = useCallback(() => {
    setSelected((prev) => {
      const next = new Map(prev);
      if (items.every((i) => prev.has(i.ticket_id))) {
        for (const i of items) next.delete(i.ticket_id);
      } else {
        for (const i of items) next.set(i.ticket_id, i);
      }
      return next;
    });
  }, [items]);

  const handleExport = useCallback(() => {
    if (selected.size === 0) {
      showError(t('请至少选择一张发票'));
      return;
    }
    if (!serviceName.trim()) {
      showError(t('请输入应税服务名称'));
      return;
    }
    const csv = generateInvoiceCSV(
      Array.from(selected.values()),
      serviceName.trim(),
    );
    const date = new Date().toISOString().slice(0, 10).replace(/-/g, '');
    downloadCSV(csv, `发票登记_${date}.csv`);
    showSuccess(t('已导出 {{count}} 张发票', { count: selected.size }));
  }, [selected, serviceName, t]);

  const columns = useMemo(
    () => [
      {
        title: (
          <Checkbox checked={allOnPageSelected} onChange={toggleAll} />
        ),
        dataIndex: '_select',
        width: 48,
        render: (_, record) => (
          <Checkbox
            checked={selected.has(record.ticket_id)}
            onChange={() => toggleItem(record)}
          />
        ),
      },
      {
        title: 'ID',
        dataIndex: 'ticket_id',
        width: 70,
        render: (v) => <span style={{ fontFamily: 'monospace', fontSize: 12 }}>#{v}</span>,
      },
      {
        title: t('公司名称'),
        dataIndex: 'company_name',
        ellipsis: true,
        width: 160,
      },
      {
        title: t('邮箱'),
        dataIndex: 'email',
        ellipsis: true,
        width: 160,
      },
      {
        title: t('金额'),
        dataIndex: 'total_money',
        width: 100,
        align: 'right',
        render: (v) => <span style={{ fontFamily: 'monospace', fontSize: 12 }}>¥{v.toFixed(2)}</span>,
      },
      {
        title: t('订单数'),
        dataIndex: 'order_count',
        width: 70,
        align: 'center',
      },
      {
        title: t('状态'),
        dataIndex: 'status',
        width: 80,
        render: (v) => statusLabel(v, t),
      },
      {
        title: t('创建时间'),
        dataIndex: 'created_time',
        width: 120,
        render: (v) => (
          <Typography.Text type='tertiary' size='small'>
            {timestamp2string(v)}
          </Typography.Text>
        ),
      },
    ],
    [t, allOnPageSelected, toggleAll, selected, toggleItem],
  );

  const datePresets = useMemo(() => [
    {
      text: t('今天'),
      start: (() => { const d = new Date(); d.setHours(0,0,0,0); return d; })(),
      end: new Date(),
    },
    {
      text: t('近 7 天'),
      start: (() => { const d = new Date(); d.setDate(d.getDate() - 6); d.setHours(0,0,0,0); return d; })(),
      end: new Date(),
    },
    {
      text: t('本周'),
      start: (() => { const d = new Date(); d.setDate(d.getDate() - d.getDay()); d.setHours(0,0,0,0); return d; })(),
      end: new Date(),
    },
    {
      text: t('近 30 天'),
      start: (() => { const d = new Date(); d.setDate(d.getDate() - 29); d.setHours(0,0,0,0); return d; })(),
      end: new Date(),
    },
    {
      text: t('本月'),
      start: (() => { const d = new Date(); d.setDate(1); d.setHours(0,0,0,0); return d; })(),
      end: new Date(),
    },
  ], [t]);

  const statusOpts = useMemo(
    () =>
      TICKET_STATUS_OPTIONS.map((o) => ({
        value: o.value,
        label: t(o.label),
      })),
    [t],
  );

  return (
    <Modal
      title={t('导出发票列表')}
      visible={visible}
      onCancel={onClose}
      centered
      width={860}
      style={{ maxWidth: '92vw' }}
      bodyStyle={{
        maxHeight: 'calc(80vh - 120px)',
        overflowY: 'auto',
        overflowX: 'hidden',
      }}
      footer={
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 12 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
            <Typography.Text type='tertiary' size='small'>
              {t('已选择 {{count}} 张发票', { count: selected.size })}
            </Typography.Text>
            <Input
              placeholder={t('应税服务名称')}
              value={serviceName}
              onChange={setServiceName}
              style={{ width: 180, height: 32 }}
            />
          </div>
          <Button
            icon={<IconDownload />}
            theme='solid'
            onClick={handleExport}
            disabled={selected.size === 0 || !serviceName.trim()}
          >
            {t('导出')}
          </Button>
        </div>
      }
    >
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 8, marginBottom: 12 }}>
        <Input
          placeholder={t('搜索公司名称、邮箱或金额...')}
          value={keyword}
          onChange={setKeyword}
          onEnterPress={handleSearch}
          style={{ width: 200, height: 32 }}
          showClear
        />
        <Select
          value={statusFilter}
          optionList={statusOpts}
          onChange={handleStatusChange}
          style={{ width: 130 }}
          getPopupContainer={() => document.body}
        />
        <DatePicker
          type='dateTimeRange'
          value={dateRange[0] ? dateRange : undefined}
          onChange={handleDateRangeChange}
          placeholder={[t('开始时间'), t('结束时间')]}
          style={{ width: 380 }}
          showClear
          presets={datePresets}
          getPopupContainer={() => document.body}
        />
      </div>
      <Table
        columns={columns}
        dataSource={items}
        rowKey='ticket_id'
        loading={loading}
        pagination={
          total > PAGE_SIZE
            ? {
                currentPage: page,
                pageSize: PAGE_SIZE,
                total,
                onPageChange: handlePageChange,
              }
            : false
        }
        size='small'
        empty={<Typography.Text type='tertiary'>{t('暂无数据')}</Typography.Text>}
      />
    </Modal>
  );
}
