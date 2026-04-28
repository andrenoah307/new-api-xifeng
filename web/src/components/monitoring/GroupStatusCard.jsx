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

import React from 'react';
import { Card, Tag, Tooltip, Typography } from '@douyinfe/semi-ui';
import {
  AlertTriangle,
  Activity,
  Database,
  Zap,
  CircleCheck,
  CircleX,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import StatusTimeline from './StatusTimeline';

const { Text } = Typography;

function rateColor(rate) {
  if (rate == null || rate < 0) return 'var(--semi-color-text-2)';
  if (rate >= 99) return 'var(--semi-color-success)';
  if (rate >= 95) return 'var(--semi-color-success-light-active)';
  if (rate >= 80) return 'var(--semi-color-warning)';
  return 'var(--semi-color-danger)';
}

function formatFRT(ms) {
  if (ms == null || ms <= 0) return '-';
  if (ms >= 1000) return `${(ms / 1000).toFixed(2)}s`;
  return `${Math.round(ms)}ms`;
}

const Metric = ({ icon, label, value, valueColor }) => (
  <div className='rounded-xl bg-semi-color-fill-0 p-3 transition-colors group-hover:bg-semi-color-fill-1'>
    <div className='flex items-center gap-1.5 text-semi-color-text-2'>
      {icon}
      <span className='text-[10px] font-semibold uppercase tracking-wider'>
        {label}
      </span>
    </div>
    <div
      className='mt-1 font-mono text-base font-semibold leading-tight'
      style={{ color: valueColor || 'var(--semi-color-text-0)' }}
    >
      {value}
    </div>
  </div>
);

const GroupStatusCard = ({ group, onClick }) => {
  const { t } = useTranslation();

  const isOnline = group.is_online ?? group.online_channels > 0;
  const availRate =
    group.availability_rate != null && group.availability_rate >= 0
      ? group.availability_rate
      : null;
  const cacheRate =
    group.cache_hit_rate != null && group.cache_hit_rate >= 0
      ? group.cache_hit_rate
      : null;
  const showCache = cacheRate != null && cacheRate >= 3;

  // Banner: full outage or severely degraded
  const banner =
    !isOnline
      ? {
          tone: 'danger',
          label: t('分组离线'),
          desc: t('所有渠道当前不可用'),
        }
      : availRate != null && availRate < 80
        ? {
            tone: 'warning',
            label: t('可用率告警'),
            desc: t('当前可用率 {{rate}}%', { rate: availRate.toFixed(1) }),
          }
        : null;

  const bannerClass =
    banner?.tone === 'danger'
      ? 'border-b border-semi-color-danger/30 bg-semi-color-danger-light-default text-semi-color-danger'
      : banner?.tone === 'warning'
        ? 'border-b border-semi-color-warning/30 bg-semi-color-warning-light-default text-semi-color-warning'
        : '';

  return (
    <Card
      bodyStyle={{ padding: 0 }}
      className='group monitoring-card !rounded-2xl border border-semi-color-border bg-semi-color-bg-1 transition-all duration-200 hover:-translate-y-0.5 hover:shadow-md'
      style={{ cursor: onClick ? 'pointer' : 'default' }}
      onClick={() => onClick && onClick(group)}
    >
      {banner && (
        <div className={`flex items-start gap-2 px-4 py-2 ${bannerClass}`}>
          <AlertTriangle className='mt-0.5 h-3.5 w-3.5 flex-shrink-0' />
          <div className='min-w-0 flex-1'>
            <div className='text-xs font-semibold leading-tight'>
              {banner.label}
            </div>
            <div className='mt-0.5 text-[11px] leading-snug opacity-80'>
              {banner.desc}
            </div>
          </div>
        </div>
      )}

      <div className='p-4'>
        {/* Header: name + status badge */}
        <div className='mb-3 flex items-start justify-between gap-2'>
          <div className='min-w-0 flex-1'>
            <Text
              strong
              className='block truncate text-base'
              title={group.group_name}
            >
              {group.group_name}
            </Text>
            <div className='mt-0.5 flex items-center gap-1.5 text-[11px] text-semi-color-text-2'>
              {group.last_test_model && (
                <Tooltip content={group.last_test_model}>
                  <span className='max-w-[140px] truncate font-mono'>
                    {group.last_test_model}
                  </span>
                </Tooltip>
              )}
              {group.last_test_model && group.group_ratio != null && (
                <span className='text-semi-color-text-3'>·</span>
              )}
              {group.group_ratio != null && (
                <span>
                  {group.group_ratio}
                  {t('元/刀')}
                </span>
              )}
            </div>
          </div>
          <Tag
            color={isOnline ? 'green' : 'red'}
            size='small'
            shape='circle'
            type='light'
            prefixIcon={
              isOnline ? (
                <CircleCheck size={12} />
              ) : (
                <CircleX size={12} />
              )
            }
          >
            {isOnline ? t('在线') : t('离线')}
          </Tag>
        </div>

        {/* Metrics: 3 tiles for availability / FRT / cache */}
        <div className='mb-3 grid grid-cols-3 gap-2'>
          <Metric
            icon={<Activity size={12} />}
            label={t('可用率')}
            value={availRate != null ? `${availRate.toFixed(1)}%` : 'N/A'}
            valueColor={rateColor(availRate)}
          />
          <Metric
            icon={<Zap size={12} />}
            label='FRT'
            value={formatFRT(group.avg_frt ?? group.first_response_time)}
          />
          <Metric
            icon={<Database size={12} />}
            label={t('缓存')}
            value={showCache ? `${cacheRate.toFixed(1)}%` : '—'}
            valueColor={showCache ? rateColor(cacheRate) : undefined}
          />
        </div>

        {/* Status timeline */}
        {group.history && group.history.length > 0 ? (
          <StatusTimeline history={group.history} segmentCount={30} compact />
        ) : (
          <div className='flex h-[22px] items-center justify-center rounded-md bg-semi-color-fill-0 text-[10px] text-semi-color-text-3'>
            {t('暂无历史数据')}
          </div>
        )}

        {/* Footer: channel count (admin only) + last update time */}
        <div className='mt-3 flex items-center justify-between text-[11px] text-semi-color-text-2'>
          {group.total_channels != null ? (
            <span>
              {t('渠道')}{' '}
              <span className='font-semibold text-semi-color-text-1'>
                {group.online_channels ?? 0}
              </span>
              <span className='text-semi-color-text-3'>
                {' '}
                / {group.total_channels}
              </span>
            </span>
          ) : (
            <span />
          )}
          {group.updated_at > 0 && (
            <span className='font-mono'>
              {new Date(group.updated_at * 1000).toLocaleTimeString([], {
                hour: '2-digit',
                minute: '2-digit',
                hour12: false,
              })}
            </span>
          )}
        </div>
      </div>
    </Card>
  );
};

export default GroupStatusCard;
