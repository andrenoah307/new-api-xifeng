import React, { useEffect, useState, useMemo } from 'react';
import {
  SideSheet,
  Table,
  Tag,
  Typography,
  Skeleton,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, isAdmin } from '../../helpers';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import AvailabilityCacheChart from './AvailabilityCacheChart';

const { Text } = Typography;

function getRateColor(rate) {
  if (rate >= 95) return 'green';
  if (rate >= 90) return 'lime';
  if (rate >= 85) return 'yellow';
  if (rate >= 75) return 'orange';
  return 'red';
}

function formatFRT(ms) {
  if (ms == null || ms <= 0) return '-';
  return (ms / 1000).toFixed(2) + 's';
}

function formatTime(ts) {
  if (!ts) return '-';
  const d = new Date(ts);
  if (isNaN(d.getTime())) return '-';
  return d.toLocaleString([], {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  });
}

const GroupDetailPanel = ({ visible, group, onClose }) => {
  const { t } = useTranslation();
  const isMobile = useIsMobile();
  const [detail, setDetail] = useState(null);
  const [history, setHistory] = useState([]);
  const [loading, setLoading] = useState(false);
  const [intervalMinutes, setIntervalMinutes] = useState(5);

  useEffect(() => {
    if (!visible || !group) return;
    setLoading(true);

    const admin = isAdmin();
    const groupName = group.group_name;

    const fetchDetail = async () => {
      try {
        if (admin) {
          const res = await API.get(
            `/api/monitoring/admin/groups/${encodeURIComponent(groupName)}`
          );
          if (res.data.success) {
            setDetail(res.data.data);
          } else {
            showError(res.data.message || t('获取分组详情失败'));
          }
        }
      } catch {
        showError(t('获取分组详情失败'));
      }
    };

    const fetchHistory = async () => {
      try {
        const prefix = admin ? 'admin' : 'public';
        const res = await API.get(
          `/api/monitoring/${prefix}/groups/${encodeURIComponent(groupName)}/history`
        );
        if (res.data.success) {
          const data = res.data.data;
          setHistory(data.history || data || []);
          if (data.aggregation_interval_minutes) {
            setIntervalMinutes(data.aggregation_interval_minutes);
          }
        }
      } catch {
        showError(t('获取历史数据失败'));
      }
    };

    Promise.all([fetchDetail(), fetchHistory()]).finally(() =>
      setLoading(false)
    );
  }, [visible, group, t]);

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
        ellipsis: true,
      },
      {
        title: t('状态'),
        dataIndex: 'enabled',
        width: 80,
        render: (enabled) => (
          <Tag color={enabled ? 'green' : 'grey'} size='small'>
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
          <Tag color={getRateColor(rate ?? 0)} size='small'>
            {rate != null ? `${rate.toFixed(1)}%` : '-'}
          </Tag>
        ),
      },
      {
        title: t('缓存命中率'),
        dataIndex: 'cache_hit_rate',
        width: 100,
        sorter: (a, b) =>
          (a.cache_hit_rate ?? 0) - (b.cache_hit_rate ?? 0),
        render: (rate) => (
          <Tag
            color={rate != null && rate >= 3 ? getRateColor(rate) : 'grey'}
            size='small'
          >
            {rate != null && rate >= 3
              ? `${rate.toFixed(1)}%`
              : t('尚未获取')}
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
        ellipsis: true,
      },
      {
        title: t('最后测试'),
        dataIndex: 'last_test_time',
        width: 130,
        render: (ts) => (
          <Text size='small' type='tertiary'>
            {formatTime(ts)}
          </Text>
        ),
      },
    ],
    [t]
  );

  const channelData = useMemo(() => {
    if (!detail || !detail.channel_stats) return [];
    return detail.channel_stats.map((ch, idx) => ({
      key: ch.channel_id || idx,
      ...ch,
    }));
  }, [detail]);

  return (
    <SideSheet
      visible={visible}
      onCancel={onClose}
      placement='right'
      width={isMobile ? '100%' : 640}
      title={
        group
          ? `${group.group_name} - ${t('分组详情')}`
          : t('分组详情')
      }
      bodyStyle={{ padding: '16px 20px' }}
    >
      {/* History chart */}
      <div style={{ marginBottom: 24 }}>
        <Text strong style={{ fontSize: 14, marginBottom: 8, display: 'block' }}>
          {t('历史趋势')}
        </Text>
        {loading ? (
          <Skeleton.Paragraph rows={6} style={{ height: 260 }} />
        ) : (
          <AvailabilityCacheChart
            history={history}
            intervalMinutes={intervalMinutes}
          />
        )}
      </div>

      {/* Channel details table (admin only) */}
      {isAdmin() && (
        <div>
          <Text
            strong
            style={{ fontSize: 14, marginBottom: 8, display: 'block' }}
          >
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
              empty={
                <Text type='tertiary'>{t('暂无渠道数据')}</Text>
              }
            />
          )}
        </div>
      )}
    </SideSheet>
  );
};

export default GroupDetailPanel;
