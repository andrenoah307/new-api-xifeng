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

import React, { useEffect, useMemo, useState } from 'react';
import {
  SideSheet,
  Skeleton,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { Activity, Database, Zap } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API, isAdmin, showError } from '../../helpers';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import StatusTimeline from './StatusTimeline';
import AvailabilityCacheChart from './AvailabilityCacheChart';

const { Text } = Typography;

function rateColor(rate) {
  if (rate == null || rate < 0) return 'var(--semi-color-text-2)';
  if (rate >= 99) return 'var(--semi-color-success)';
  if (rate >= 95) return 'var(--semi-color-success-light-active)';
  if (rate >= 80) return 'var(--semi-color-warning)';
  return 'var(--semi-color-danger)';
}

function rateTagColor(rate) {
  if (rate == null) return 'grey';
  if (rate >= 99) return 'green';
  if (rate >= 95) return 'lime';
  if (rate >= 80) return 'yellow';
  return 'red';
}

function formatFRT(ms) {
  if (ms == null || ms <= 0) return '-';
  if (ms >= 1000) return `${(ms / 1000).toFixed(2)}s`;
  return `${Math.round(ms)}ms`;
}

const Stat = ({ icon, label, value, valueColor }) => (
  <div className='rounded-xl border border-semi-color-border bg-semi-color-bg-1 p-3'>
    <div className='flex items-center gap-1.5 text-semi-color-text-2'>
      {icon}
      <span className='text-[10px] font-semibold uppercase tracking-wider'>
        {label}
      </span>
    </div>
    <div
      className='mt-1.5 font-mono text-xl font-semibold leading-none'
      style={{ color: valueColor || 'var(--semi-color-text-0)' }}
    >
      {value}
    </div>
  </div>
);

const GroupDetailPanel = ({ visible, group, onClose }) => {
  const { t } = useTranslation();
  const isMobile = useIsMobile();
  const [detail, setDetail] = useState(null);
  const [history, setHistory] = useState([]);
  const [loading, setLoading] = useState(false);
  const [intervalMinutes, setIntervalMinutes] = useState(5);
  const admin = isAdmin();

  useEffect(() => {
    if (!visible || !group) return;
    setLoading(true);
    const groupName = group.group_name;
    const prefix = admin ? 'admin' : 'public';

    const tasks = [
      admin
        ? API.get(`/api/monitoring/admin/groups/${encodeURIComponent(groupName)}`)
            .then((res) => {
              if (res.data.success) setDetail(res.data.data);
              else showError(res.data.message || t('获取分组详情失败'));
            })
            .catch(() => showError(t('获取分组详情失败')))
        : Promise.resolve(),
      API.get(
        `/api/monitoring/${prefix}/groups/${encodeURIComponent(groupName)}/history`,
      )
        .then((res) => {
          if (res.data.success) {
            const data = res.data.data;
            setHistory(data?.history || data || []);
            if (data?.aggregation_interval_minutes) {
              setIntervalMinutes(data.aggregation_interval_minutes);
            }
          }
        })
        .catch(() => {
          /* silent */
        }),
    ];
    Promise.all(tasks).finally(() => setLoading(false));
  }, [visible, group, t, admin]);

  const isOnline = group
    ? group.is_online ?? group.online_channels > 0
    : false;
  const availRate =
    group?.availability_rate != null && group.availability_rate >= 0
      ? group.availability_rate
      : null;
  const cacheRate =
    group?.cache_hit_rate != null && group.cache_hit_rate >= 0
      ? group.cache_hit_rate
      : null;
  const showCache = cacheRate != null && cacheRate >= 3;

  const channelColumns = useMemo(
    () => [
      {
        title: t('渠道ID'),
        dataIndex: 'channel_id',
        width: 80,
      },
      {
        title: t('渠道名称'),
        dataIndex: 'channel_name',
        width: 140,
        ellipsis: { showTitle: true },
      },
      {
        title: t('状态'),
        dataIndex: 'enabled',
        width: 80,
        render: (enabled) => (
          <Tag color={enabled ? 'green' : 'grey'} size='small' shape='circle'>
            {enabled ? t('启用') : t('禁用')}
          </Tag>
        ),
      },
      {
        title: t('可用率'),
        dataIndex: 'availability_rate',
        width: 100,
        defaultSortOrder: 'ascend',
        sorter: (a, b) =>
          (a.availability_rate ?? 0) - (b.availability_rate ?? 0),
        render: (rate) => (
          <Tag color={rateTagColor(rate)} size='small' shape='circle'>
            {rate != null ? `${rate.toFixed(1)}%` : '-'}
          </Tag>
        ),
      },
      {
        title: t('缓存命中率'),
        dataIndex: 'cache_hit_rate',
        width: 100,
        sorter: (a, b) => (a.cache_hit_rate ?? 0) - (b.cache_hit_rate ?? 0),
        render: (rate) => (
          <Tag
            color={rate != null && rate >= 3 ? rateTagColor(rate) : 'grey'}
            size='small'
            shape='circle'
          >
            {rate != null && rate >= 3 ? `${rate.toFixed(1)}%` : '—'}
          </Tag>
        ),
      },
      {
        title: 'FRT',
        dataIndex: 'first_response_time',
        width: 90,
        sorter: (a, b) =>
          (a.first_response_time ?? 0) - (b.first_response_time ?? 0),
        render: (ms) => <Text size='small'>{formatFRT(ms)}</Text>,
      },
      {
        title: t('测试模型'),
        dataIndex: 'test_model',
        width: 140,
        ellipsis: { showTitle: true },
      },
      {
        title: t('最后测试'),
        dataIndex: 'last_test_time',
        width: 130,
        render: (ts) => (
          <Text size='small' type='tertiary'>
            {ts ? new Date(ts).toLocaleString([], { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }) : '-'}
          </Text>
        ),
      },
    ],
    [t],
  );

  const channelData = useMemo(() => {
    if (!detail || !detail.channel_stats) return [];
    return detail.channel_stats.map((ch, idx) => ({
      key: ch.channel_id || idx,
      ...ch,
    }));
  }, [detail]);

  const title = group
    ? (
        <div className='flex items-center gap-2'>
          <Tag
            color={isOnline ? 'green' : 'red'}
            shape='circle'
            type='light'
            size='small'
          >
            {isOnline ? t('在线') : t('离线')}
          </Tag>
          <span className='font-medium'>{group.group_name}</span>
          <Text type='tertiary' size='small'>
            {t('分组详情')}
          </Text>
        </div>
      )
    : t('分组详情');

  return (
    <SideSheet
      visible={visible}
      onCancel={onClose}
      placement='right'
      width={isMobile ? '100%' : 720}
      title={title}
      bodyStyle={{ padding: 0 }}
    >
      <div className='space-y-5 p-4 sm:p-5'>
        {/* Hero metrics */}
        <div className='grid grid-cols-3 gap-3'>
          <Stat
            icon={<Activity size={12} />}
            label={t('可用率')}
            value={availRate != null ? `${availRate.toFixed(1)}%` : 'N/A'}
            valueColor={rateColor(availRate)}
          />
          <Stat
            icon={<Zap size={12} />}
            label='FRT'
            value={formatFRT(group?.avg_frt ?? group?.first_response_time)}
          />
          <Stat
            icon={<Database size={12} />}
            label={t('缓存命中率')}
            value={showCache ? `${cacheRate.toFixed(1)}%` : '—'}
            valueColor={showCache ? rateColor(cacheRate) : undefined}
          />
        </div>

        {/* Status timeline */}
        <div>
          <Text
            strong
            className='!mb-2 block text-sm'
          >
            {t('状态时间线')}
          </Text>
          {history.length > 0 ? (
            <StatusTimeline history={history} segmentCount={60} />
          ) : (
            <div className='flex h-12 items-center justify-center rounded-md border border-dashed border-semi-color-border text-xs text-semi-color-text-2'>
              {t('暂无历史数据')}
            </div>
          )}
        </div>

        {/* Trend chart */}
        {history.length > 0 && (
          <div>
            <Text strong className='!mb-2 block text-sm'>
              {t('趋势')}
            </Text>
            <div className='rounded-xl border border-semi-color-border bg-semi-color-bg-1 p-3'>
              <AvailabilityCacheChart
                history={history}
                intervalMinutes={intervalMinutes}
              />
            </div>
          </div>
        )}

        {/* Channel table (admin only) */}
        {admin && (
          <div>
            <Text strong className='!mb-2 block text-sm'>
              {t('渠道详情')}
            </Text>
            {loading ? (
              <Skeleton.Paragraph rows={4} />
            ) : (
              <Table
                columns={channelColumns}
                dataSource={channelData}
                pagination={false}
                size='small'
                scroll={{ x: 800 }}
                empty={<Text type='tertiary'>{t('暂无渠道数据')}</Text>}
              />
            )}
          </div>
        )}
      </div>
    </SideSheet>
  );
};

export default GroupDetailPanel;
