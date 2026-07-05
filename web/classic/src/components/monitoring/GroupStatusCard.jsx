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
import { Tooltip, Typography } from '@douyinfe/semi-ui';
import { Database, Zap } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import StatusTimeline from './StatusTimeline';

const { Text } = Typography;

function rateAccent(rate) {
  if (rate == null || rate < 0) return 'var(--semi-color-text-2)';
  if (rate >= 99) return 'var(--semi-color-success)';
  if (rate >= 95) return 'var(--semi-color-success-light-active)';
  if (rate >= 80) return 'var(--semi-color-warning)';
  return 'var(--semi-color-danger)';
}

function formatFRT(ms) {
  if (ms == null || ms <= 0) return '—';
  if (ms >= 1000) return `${(ms / 1000).toFixed(2)}s`;
  return `${Math.round(ms)}ms`;
}

function formatClock(unixSec) {
  if (!unixSec || unixSec <= 0) return '';
  return new Date(unixSec * 1000).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  });
}

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
  const frt = group.avg_frt ?? group.first_response_time;

  // Status dot reflects current online state primarily, then degrades
  // by historical availability if online.
  const dotColor = !isOnline
    ? 'var(--semi-color-danger)'
    : availRate == null
      ? 'var(--semi-color-fill-2)'
      : rateAccent(availRate);

  const headlineColor = !isOnline
    ? 'var(--semi-color-danger)'
    : rateAccent(availRate);

  return (
    <div
      className='group relative rounded-2xl border border-semi-color-border bg-semi-color-bg-1 p-5 transition-all duration-200 hover:-translate-y-0.5 hover:border-semi-color-primary/40 hover:shadow-[0_8px_24px_-12px_rgba(0,0,0,0.12)]'
      style={{ cursor: onClick ? 'pointer' : 'default' }}
      onClick={() => onClick && onClick(group)}
    >
      {/* Header: name + meta on left, big availability on right */}
      <div className='flex items-start justify-between gap-3'>
        <div className='min-w-0 flex-1'>
          <div className='flex items-center gap-2'>
            <span
              className={`inline-block h-2 w-2 flex-shrink-0 rounded-full ${
                !isOnline ? 'animate-pulse' : ''
              }`}
              style={{ background: dotColor }}
              aria-hidden
            />
            <Text
              strong
              className='!block truncate !text-sm !text-semi-color-text-0'
              title={group.group_name}
            >
              {group.group_name}
            </Text>
          </div>
          <div className='mt-1.5 flex items-center gap-1.5 text-[11px] text-semi-color-text-3'>
            {group.last_test_model && (
              <Tooltip content={group.last_test_model}>
                <span className='max-w-[140px] truncate font-mono'>
                  {group.last_test_model}
                </span>
              </Tooltip>
            )}
            {group.last_test_model && group.group_ratio != null && (
              <span className='opacity-60'>·</span>
            )}
            {group.group_ratio != null && (
              <span>
                {group.group_ratio}
                {t('元/刀')}
              </span>
            )}
          </div>
        </div>

        <div className='flex-shrink-0 text-right leading-none'>
          {availRate != null ? (
            <div
              className='font-mono text-[28px] font-semibold tracking-tight'
              style={{ color: headlineColor }}
            >
              {availRate.toFixed(1)}
              <span className='ml-0.5 text-base font-normal'>%</span>
            </div>
          ) : (
            <div className='font-mono text-[28px] font-semibold tracking-tight text-semi-color-text-3'>
              —
            </div>
          )}
          <div className='mt-1 text-[10px] uppercase tracking-wider text-semi-color-text-3'>
            {isOnline ? t('可用率') : t('已离线')}
          </div>
        </div>
      </div>

      {/* Status timeline — the focal proof */}
      <div className='mt-5'>
        {group.history && group.history.length > 0 ? (
          <StatusTimeline history={group.history} segmentCount={32} compact />
        ) : (
          <div className='flex h-[22px] items-center justify-center rounded-md bg-semi-color-fill-0 text-[10px] text-semi-color-text-3'>
            {t('暂无历史数据')}
          </div>
        )}
      </div>

      {/* Footer: inline stats, no nested boxes */}
      <div className='mt-4 flex items-center justify-between text-[11px] text-semi-color-text-2'>
        <div className='flex items-center gap-3'>
          <span className='inline-flex items-center gap-1'>
            <Zap size={11} className='text-semi-color-text-3' />
            <span className='font-mono'>{formatFRT(frt)}</span>
          </span>
          <span className='inline-flex items-center gap-1'>
            <Database size={11} className='text-semi-color-text-3' />
            <span className='font-mono'>
              {showCache ? `${cacheRate.toFixed(1)}%` : '—'}
            </span>
          </span>
        </div>
        <div className='flex items-center gap-3'>
          {group.total_channels != null && (
            <span>
              <span className='font-mono text-semi-color-text-1'>
                {group.online_channels ?? 0}
              </span>
              <span className='text-semi-color-text-3'>
                /{group.total_channels}
              </span>
            </span>
          )}
          {group.updated_at > 0 && (
            <span className='font-mono text-semi-color-text-3'>
              {formatClock(group.updated_at)}
            </span>
          )}
        </div>
      </div>
    </div>
  );
};

export default GroupStatusCard;
