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

import React, { useMemo } from 'react';
import { Tooltip } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

/**
 * Color mapping for availability segments. Uses Semi design tokens so dark
 * mode is automatic via `theme-mode=dark` on body.
 */
function segmentColor(rate) {
  if (rate == null || rate < 0)
    return 'var(--semi-color-fill-1)';
  if (rate >= 99) return 'var(--semi-color-success)';
  if (rate >= 95) return 'var(--semi-color-success-light-active)';
  if (rate >= 80) return 'var(--semi-color-warning)';
  if (rate >= 50) return '#f97316';
  return 'var(--semi-color-danger)';
}

function segmentLabel(rate, t) {
  if (rate == null || rate < 0) return t('暂无数据');
  if (rate >= 99) return t('正常');
  if (rate >= 95) return t('轻微抖动');
  if (rate >= 80) return t('部分异常');
  if (rate >= 50) return t('严重异常');
  return t('故障');
}

function formatTime(unixSec) {
  if (!unixSec) return '-';
  return new Date(unixSec * 1000).toLocaleString([], {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  });
}

/**
 * Right-aligned, fixed-segment status bar. Latest sample sits on the
 * right; older samples drift left. Empty slots render muted so the bar
 * keeps a stable visual width even when history is short.
 *
 * Props:
 *   history: [{ recorded_at, availability_rate, ... }]
 *   segmentCount: how many slots to render (default 60)
 *   compact: smaller height, no labels
 */
const StatusTimeline = ({ history, segmentCount = 60, compact = false }) => {
  const { t } = useTranslation();

  const segments = useMemo(() => {
    const sorted = (history || [])
      .filter((h) => h && typeof h.recorded_at === 'number')
      .sort((a, b) => a.recorded_at - b.recorded_at);
    // Take the most recent N
    const slice = sorted.slice(-segmentCount);
    // Pad on the LEFT with nulls so latest is right-aligned
    const pad = Array(Math.max(0, segmentCount - slice.length)).fill(null);
    return [...pad, ...slice];
  }, [history, segmentCount]);

  const filled = segments.filter((s) => s != null).length;
  const height = compact ? 22 : 32;

  return (
    <div className='space-y-1.5'>
      {!compact && (
        <div className='flex items-center justify-between text-[10px] uppercase tracking-wider text-semi-color-text-2'>
          <span>
            {filled <= 1
              ? t('历史 (最新)')
              : t('历史 ({{n}} 条)', { n: filled })}
          </span>
          <span className='opacity-60'>{t('从左到右:旧 → 新')}</span>
        </div>
      )}
      <div
        className='flex w-full overflow-hidden rounded-md bg-semi-color-fill-0'
        style={{ height, gap: 2, padding: 2 }}
      >
        {segments.map((seg, idx) => {
          const rate = seg?.availability_rate;
          const bg = segmentColor(rate);
          const isEmpty = seg == null;
          const tip = isEmpty ? (
            <span className='text-xs'>{t('暂无数据')}</span>
          ) : (
            <div className='space-y-0.5 text-xs'>
              <div className='font-medium'>{formatTime(seg.recorded_at)}</div>
              <div>
                {t('状态')}: {segmentLabel(rate, t)}
              </div>
              {rate != null && rate >= 0 && (
                <div>
                  {t('可用率')}: {rate.toFixed(1)}%
                </div>
              )}
              {seg.cache_hit_rate != null && seg.cache_hit_rate >= 0 && (
                <div>
                  {t('缓存命中率')}: {seg.cache_hit_rate.toFixed(1)}%
                </div>
              )}
            </div>
          );
          return (
            <Tooltip key={idx} content={tip} position='top' trigger='hover'>
              <div
                className='flex-1 rounded-sm transition-opacity hover:opacity-80'
                style={{
                  background: bg,
                  opacity: isEmpty ? 0.35 : 1,
                  minWidth: 2,
                }}
              />
            </Tooltip>
          );
        })}
      </div>
    </div>
  );
};

export default StatusTimeline;
