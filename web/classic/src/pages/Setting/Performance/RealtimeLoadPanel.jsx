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

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Banner,
  Button,
  Col,
  Collapse,
  Descriptions,
  Form,
  Input,
  Progress,
  Row,
  Select,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { API } from '../../../helpers';
import { useTranslation } from 'react-i18next';

const { Text } = Typography;

const POLL_INTERVAL_MS = 10000;

function formatBytes(bytes) {
  if (!bytes || isNaN(bytes) || bytes <= 0) return '0 B';
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(
    sizes.length - 1,
    Math.floor(Math.log(bytes) / Math.log(1024)),
  );
  return parseFloat((bytes / Math.pow(1024, i)).toFixed(1)) + ' ' + sizes[i];
}

function formatCompact(value) {
  if (!Number.isFinite(value)) return '0';
  if (Math.abs(value) < 1000) return String(value);
  if (Math.abs(value) < 1000000) return (value / 1000).toFixed(1) + 'k';
  return (value / 1000000).toFixed(1) + 'M';
}

function errorRate(requests, errors) {
  if (!requests || requests <= 0) return 0;
  return (errors / requests) * 100;
}

// A dependency-free sparkline: this panel is one settings section, so a charting
// library would cost more than the feature.
function Sparkline({ values, color }) {
  if (!values || values.length === 0) return null;
  const width = 100;
  const height = 28;
  const max = Math.max(1, ...values);
  const step = values.length > 1 ? width / (values.length - 1) : width;
  const points = values
    .map((value, index) => {
      const x = index * step;
      const y = height - (value / max) * height;
      return `${x.toFixed(2)},${y.toFixed(2)}`;
    })
    .join(' ');
  return (
    <svg
      viewBox={`0 0 ${width} ${height}`}
      preserveAspectRatio='none'
      style={{ width: '100%', height: 28 }}
    >
      <polyline
        points={points}
        fill='none'
        stroke={color}
        strokeWidth='1.5'
        vectorEffect='non-scaling-stroke'
      />
    </svg>
  );
}

export default function RealtimeLoadPanel() {
  const { t } = useTranslation();
  const [snapshot, setSnapshot] = useState(null);
  const [unreachable, setUnreachable] = useState(false);
  const [channelFilter, setChannelFilter] = useState('');
  const [sortKey, setSortKey] = useState('concurrency');
  const inFlightRef = useRef(false);

  const fetchSnapshot = useCallback(async () => {
    // A slow poll must not stack up behind itself: that is exactly the load this
    // panel exists to warn about.
    if (inFlightRef.current) return;
    inFlightRef.current = true;
    try {
      // The endpoint answers 200 even when degraded, but a network drop would
      // otherwise raise a toast on every tick.
      const res = await API.get('/api/performance/realtime', {
        skipErrorHandler: true,
      });
      if (res.data?.success) {
        setSnapshot(res.data.data);
        setUnreachable(false);
      }
    } catch (error) {
      setUnreachable(true);
    } finally {
      inFlightRef.current = false;
    }
  }, []);

  useEffect(() => {
    fetchSnapshot();
    let timer = null;
    const start = () => {
      if (timer !== null) return;
      timer = setInterval(fetchSnapshot, POLL_INTERVAL_MS);
    };
    const stop = () => {
      if (timer === null) return;
      clearInterval(timer);
      timer = null;
    };
    // Polling a hidden tab every ten seconds costs the server for nobody.
    const onVisibility = () => {
      if (document.visibilityState === 'visible') {
        fetchSnapshot();
        start();
      } else {
        stop();
      }
    };
    if (document.visibilityState === 'visible') start();
    document.addEventListener('visibilitychange', onVisibility);
    return () => {
      stop();
      document.removeEventListener('visibilitychange', onVisibility);
    };
  }, [fetchSnapshot]);

  const series = snapshot?.series || [];
  const totals = snapshot?.totals;

  const windowTotals = useMemo(() => {
    return series.reduce(
      (acc, point) => ({
        requests: acc.requests + point.requests,
        success: acc.success + point.success,
        errors: acc.errors + point.errors,
        clientGone: acc.clientGone + point.client_gone,
        promptTokens: acc.promptTokens + point.prompt_tokens,
        completionTokens: acc.completionTokens + point.completion_tokens,
        rejGate: acc.rejGate + point.rej_gate,
        rejConcurrency: acc.rejConcurrency + point.rej_concurrency,
        rejBody: acc.rejBody + point.rej_body,
        rejMemory: acc.rejMemory + point.rej_memory,
        rejModelRPM: acc.rejModelRPM + point.rej_model_rpm,
        rejUserRPM: acc.rejUserRPM + point.rej_user_rpm,
      }),
      {
        requests: 0,
        success: 0,
        errors: 0,
        clientGone: 0,
        promptTokens: 0,
        completionTokens: 0,
        rejGate: 0,
        rejConcurrency: 0,
        rejBody: 0,
        rejMemory: 0,
        rejModelRPM: 0,
        rejUserRPM: 0,
      },
    );
  }, [series]);

  const currentRpm = series.length > 0 ? series[series.length - 1].requests : 0;
  const concurrencyPercent =
    totals && totals.max_concurrent > 0
      ? Math.min(
          100,
          Math.round((totals.active_requests / totals.max_concurrent) * 100),
        )
      : 0;
  const memoryPercent = totals ? totals.cgroup_permille / 10 : 0;

  const visibleChannels = useMemo(() => {
    const keyword = channelFilter.trim().toLowerCase();
    const filtered = (snapshot?.channels || []).filter((channel) => {
      if (keyword === '') return true;
      return (
        String(channel.channel_id).includes(keyword) ||
        (channel.channel_name || '').toLowerCase().includes(keyword)
      );
    });
    return [...filtered].sort((a, b) => {
      if (sortKey === 'error_rate') {
        const diff =
          errorRate(b.requests, b.errors) - errorRate(a.requests, a.errors);
        if (diff !== 0) return diff;
      } else if (a[sortKey] !== b[sortKey]) {
        return b[sortKey] - a[sortKey];
      }
      return a.channel_id - b.channel_id;
    });
  }, [snapshot, channelFilter, sortKey]);

  const channelWindowMinutes =
    (snapshot?.channels?.[0]?.window_secs || 300) / 60;

  const channelColumns = [
    {
      title: t('渠道'),
      dataIndex: 'channel_name',
      render: (name, record) => (
        <span>
          <Text strong>{name || t('未知')}</Text>
          <Text type='tertiary'> #{record.channel_id}</Text>
        </span>
      ),
    },
    { title: t('并发数'), dataIndex: 'concurrency', align: 'right' },
    {
      title: t('请求数'),
      dataIndex: 'requests',
      align: 'right',
      render: (value) => formatCompact(value),
    },
    {
      title: t('错误数'),
      dataIndex: 'errors',
      align: 'right',
      render: (value) => formatCompact(value),
    },
    {
      title: t('错误率'),
      dataIndex: 'error_rate',
      align: 'right',
      render: (_value, record) => {
        const rate = errorRate(record.requests, record.errors);
        const text = rate.toFixed(1) + '%';
        return rate >= 20 ? <Tag color='red'>{text}</Tag> : text;
      },
    },
  ];

  const instanceColumns = [
    {
      title: t('节点'),
      dataIndex: 'node',
      render: (node, record) => (
        <span>
          <Text strong>{node}</Text>
          {record.cgroup_tripped && (
            <Tag color='red' style={{ marginLeft: 8 }}>
              {t('已跳闸')}
            </Tag>
          )}
        </span>
      ),
    },
    {
      title: t('活跃请求'),
      dataIndex: 'active_requests',
      align: 'right',
      render: (value, record) => `${value} / ${record.max_concurrent}`,
    },
    {
      title: t('请求体占用'),
      dataIndex: 'active_body_bytes',
      align: 'right',
      render: (value) => formatBytes(value),
    },
    {
      title: t('内存压力'),
      dataIndex: 'cgroup_permille',
      align: 'right',
      render: (value) => (value / 10).toFixed(1) + '%',
    },
    { title: 'Goroutines', dataIndex: 'goroutines', align: 'right' },
    {
      title: t('最后上报'),
      dataIndex: 'stale_seconds',
      align: 'right',
      render: (value) => `${value}s`,
    },
  ];

  const cardStyle = {
    padding: 16,
    background: 'var(--semi-color-fill-0)',
    borderRadius: 8,
    flex: 1,
  };

  return (
    <Form.Section text={t('实时负载')}>
      <Banner
        type='info'
        description={t(
          '所有实例的实时中继负载，每 10 秒刷新一次。计数来自 Redis，覆盖最近 60 分钟，不写入数据库。',
        )}
        style={{ marginBottom: 16 }}
      />

      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col span={24}>
          <div
            style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}
          >
            <Button onClick={fetchSnapshot}>{t('刷新统计')}</Button>
            {snapshot && (
              <Text type='tertiary'>
                {t('实例数')}: {snapshot.instances.length}
              </Text>
            )}
          </div>
        </Col>
      </Row>

      {unreachable && (
        <Banner
          type='danger'
          title={t('实时数据不可用')}
          description={t('无法连接服务器，正在自动重试。')}
          style={{ marginBottom: 16 }}
        />
      )}

      {snapshot?.degraded && (
        <Banner
          type='warning'
          title={t('仅显示当前实例')}
          description={
            snapshot.warning === 'redis_disabled'
              ? t('未启用 Redis，跨实例汇总与 60 分钟历史不可用。')
              : t('无法连接 Redis，跨实例汇总与 60 分钟历史不可用。')
          }
          style={{ marginBottom: 16 }}
        />
      )}

      {snapshot && (
        <>
          <Row gutter={16} style={{ marginBottom: 16 }}>
            <Col xs={24} md={8} style={{ display: 'flex', marginBottom: 8 }}>
              <div style={cardStyle}>
                <Text strong style={{ display: 'block', marginBottom: 8 }}>
                  {t('活跃请求 / 上限')}
                </Text>
                <Text style={{ fontSize: 24 }}>
                  {totals?.active_requests ?? 0} / {totals?.max_concurrent ?? 0}
                </Text>
                <Progress
                  percent={concurrencyPercent}
                  showInfo
                  style={{ margin: '8px 0' }}
                  stroke={
                    concurrencyPercent > 80
                      ? 'var(--semi-color-danger)'
                      : 'var(--semi-color-primary)'
                  }
                />
                <Text type='tertiary'>
                  {t('请求体占用')}:{' '}
                  {formatBytes(totals?.active_body_bytes ?? 0)} /{' '}
                  {formatBytes(totals?.max_body_bytes ?? 0)}
                </Text>
              </div>
            </Col>
            <Col xs={24} md={8} style={{ display: 'flex', marginBottom: 8 }}>
              <div style={cardStyle}>
                <Text strong style={{ display: 'block', marginBottom: 8 }}>
                  {t('每分钟请求数')}
                </Text>
                <Text style={{ fontSize: 24 }}>{currentRpm}</Text>
                <Sparkline
                  values={series.map((point) => point.requests)}
                  color='var(--semi-color-primary)'
                />
                <Text type='tertiary'>
                  {t('最近 60 分钟')}: {formatCompact(windowTotals.requests)}
                </Text>
              </div>
            </Col>
            <Col xs={24} md={8} style={{ display: 'flex', marginBottom: 8 }}>
              <div style={cardStyle}>
                <Text strong style={{ display: 'block', marginBottom: 8 }}>
                  {t('内存压力')}
                </Text>
                <div
                  style={{ display: 'flex', alignItems: 'center', gap: 8 }}
                >
                  <Text style={{ fontSize: 24 }}>
                    {memoryPercent.toFixed(1)}%
                  </Text>
                  <Tag color={totals?.cgroup_tripped ? 'red' : 'green'}>
                    {totals?.cgroup_tripped ? t('已跳闸') : t('正常')}
                  </Tag>
                </div>
                <Progress
                  percent={Math.min(100, Math.round(memoryPercent))}
                  showInfo
                  style={{ margin: '8px 0' }}
                  stroke={
                    memoryPercent >= 75
                      ? 'var(--semi-color-danger)'
                      : 'var(--semi-color-primary)'
                  }
                />
                <Text type='tertiary'>
                  {t('累计跳闸次数')}: {totals?.trip_count ?? 0} ·{' '}
                  {t('强制复位次数')}: {totals?.forced_reset_count ?? 0}
                </Text>
              </div>
            </Col>
          </Row>

          <Row gutter={16} style={{ marginBottom: 16 }}>
            <Col xs={24} md={12} style={{ marginBottom: 8 }}>
              <div style={cardStyle}>
                <Text strong style={{ display: 'block', marginBottom: 8 }}>
                  {t('每分钟错误数')}
                </Text>
                <Sparkline
                  values={series.map((point) => point.errors)}
                  color='var(--semi-color-danger)'
                />
                <Descriptions
                  align='left'
                  size='small'
                  style={{ marginTop: 8 }}
                  data={[
                    {
                      key: t('成功 / 失败'),
                      value: `${formatCompact(windowTotals.success)} / ${formatCompact(windowTotals.errors)}`,
                    },
                    {
                      key: t('错误率'),
                      value:
                        errorRate(
                          windowTotals.requests,
                          windowTotals.errors,
                        ).toFixed(1) + '%',
                    },
                    {
                      key: t('客户端中途断开'),
                      value: formatCompact(windowTotals.clientGone),
                    },
                    {
                      key: t('Token'),
                      value: `${formatCompact(windowTotals.promptTokens)} + ${formatCompact(windowTotals.completionTokens)}`,
                    },
                  ]}
                />
              </div>
            </Col>
            <Col xs={24} md={12} style={{ marginBottom: 8 }}>
              <div style={cardStyle}>
                <Text strong style={{ display: 'block', marginBottom: 8 }}>
                  {t('拒绝统计（最近 60 分钟）')}
                </Text>
                <Descriptions
                  align='left'
                  size='small'
                  data={[
                    {
                      key: t('并发请求超限拒绝数'),
                      value: formatCompact(windowTotals.rejConcurrency),
                    },
                    {
                      key: t('请求体预算耗尽拒绝数'),
                      value: formatCompact(windowTotals.rejBody),
                    },
                    {
                      key: t('内存压力拒绝数'),
                      value: formatCompact(windowTotals.rejMemory),
                    },
                    {
                      key: t('模型 RPM 拒绝数'),
                      value: formatCompact(windowTotals.rejModelRPM),
                    },
                    {
                      key: t('用户限流拒绝数'),
                      value: formatCompact(windowTotals.rejUserRPM),
                    },
                    {
                      key: t('其他准入拒绝数'),
                      value: formatCompact(windowTotals.rejGate),
                    },
                  ]}
                />
              </div>
            </Col>
          </Row>

          <Collapse style={{ marginBottom: 16 }}>
            <Collapse.Panel
              header={`${t('渠道并发明细')} (${snapshot.channels.length})`}
              itemKey='channels'
            >
              <div
                style={{
                  display: 'flex',
                  gap: 8,
                  flexWrap: 'wrap',
                  marginBottom: 12,
                }}
              >
                <Input
                  style={{ width: 240 }}
                  value={channelFilter}
                  onChange={setChannelFilter}
                  showClear
                  placeholder={t('按渠道名称或 ID 筛选')}
                />
                <Select
                  style={{ width: 180 }}
                  value={sortKey}
                  onChange={setSortKey}
                  prefix={t('排序')}
                >
                  <Select.Option value='concurrency'>
                    {t('并发数')}
                  </Select.Option>
                  <Select.Option value='requests'>{t('请求数')}</Select.Option>
                  <Select.Option value='errors'>{t('错误数')}</Select.Option>
                  <Select.Option value='error_rate'>
                    {t('错误率')}
                  </Select.Option>
                </Select>
              </div>
              <Table
                columns={channelColumns}
                dataSource={visibleChannels}
                rowKey='channel_id'
                pagination={false}
                size='small'
                scroll={{ y: 320 }}
                empty={t('当前窗口内没有渠道流量。')}
              />
              <Text type='tertiary' style={{ display: 'block', marginTop: 8 }}>
                {t('并发数为实时值；请求数与错误数统计最近 {{minutes}} 分钟。', {
                  minutes: channelWindowMinutes,
                })}
              </Text>
            </Collapse.Panel>
          </Collapse>

          <Row gutter={16}>
            <Col span={24}>
              <Text strong style={{ display: 'block', marginBottom: 8 }}>
                {t('实例')}
              </Text>
              <Table
                columns={instanceColumns}
                dataSource={snapshot.instances}
                rowKey='node'
                pagination={false}
                size='small'
              />
            </Col>
          </Row>
        </>
      )}
    </Form.Section>
  );
}
