import React, { useEffect, useMemo } from 'react';
import { VChart } from '@visactor/react-vchart';
import { initVChartSemiTheme } from '@visactor/vchart-semi-theme';
import { useTranslation } from 'react-i18next';

function alignAndFillHistory(history, intervalMinutes) {
  if (!history || history.length === 0) return [];

  const sorted = [...history].sort(
    (a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime()
  );

  const startMs = new Date(sorted[0].timestamp).getTime();
  const endMs = new Date(sorted[sorted.length - 1].timestamp).getTime();
  const stepMs = (intervalMinutes || 5) * 60 * 1000;

  const byTime = {};
  for (const h of sorted) {
    const t = new Date(h.timestamp).getTime();
    const aligned = Math.round(t / stepMs) * stepMs;
    byTime[aligned] = h;
  }

  const result = [];
  let lastAvail = null;
  let lastCache = null;

  for (let t = startMs; t <= endMs; t += stepMs) {
    const aligned = Math.round(t / stepMs) * stepMs;
    const entry = byTime[aligned];
    if (entry) {
      lastAvail = entry.availability_rate ?? lastAvail;
      lastCache = entry.cache_hit_rate ?? lastCache;
    }
    const timeStr = new Date(aligned).toLocaleTimeString([], {
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
    });
    if (lastAvail !== null) {
      result.push({ time: timeStr, value: lastAvail, type: 'availability' });
    }
    if (lastCache !== null) {
      result.push({ time: timeStr, value: lastCache, type: 'cache' });
    }
  }

  return result;
}

const AvailabilityCacheChart = ({ history, intervalMinutes }) => {
  const { t } = useTranslation();

  useEffect(() => {
    initVChartSemiTheme({ isWatchingThemeSwitch: true });
  }, []);

  const chartData = useMemo(
    () => alignAndFillHistory(history, intervalMinutes),
    [history, intervalMinutes]
  );

  const yMin = useMemo(() => {
    if (!chartData || chartData.length === 0) return 0;
    const vals = chartData.map((d) => d.value).filter((v) => v > 0);
    if (vals.length === 0) return 0;
    const min = Math.min(...vals);
    return Math.max(0, Math.floor(min / 5) * 5 - 5);
  }, [chartData]);

  if (!chartData || chartData.length === 0) {
    return (
      <div
        style={{
          height: 260,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: 'var(--semi-color-text-2)',
          fontSize: 14,
        }}
      >
        {t('暂无历史数据')}
      </div>
    );
  }

  const spec = {
    type: 'line',
    data: [{ id: 'history', values: chartData }],
    xField: 'time',
    yField: 'value',
    seriesField: 'type',
    height: 260,
    padding: { top: 12, bottom: 24, left: 8, right: 8 },
    line: {
      style: {
        lineWidth: 2,
        curveType: 'monotone',
      },
    },
    point: { visible: false },
    axes: [
      {
        orient: 'bottom',
        label: {
          autoRotate: true,
          autoHide: true,
          style: { fontSize: 11 },
        },
      },
      {
        orient: 'left',
        min: yMin,
        max: 100,
        label: {
          formatMethod: (v) => `${v}%`,
          style: { fontSize: 11 },
        },
      },
    ],
    legends: {
      visible: true,
      orient: 'top',
      position: 'start',
      data: [
        { label: t('可用率'), shape: { fill: '#3b82f6' } },
        { label: t('缓存命中率'), shape: { fill: '#22c55e' } },
      ],
    },
    tooltip: {
      mark: {
        content: [
          {
            key: (datum) =>
              datum.type === 'availability'
                ? t('可用率')
                : t('缓存命中率'),
            value: (datum) => `${datum.value.toFixed(1)}%`,
          },
        ],
      },
      dimension: {
        content: [
          {
            key: (datum) =>
              datum.type === 'availability'
                ? t('可用率')
                : t('缓存命中率'),
            value: (datum) => `${datum.value.toFixed(1)}%`,
          },
        ],
      },
    },
    color: ['#3b82f6', '#22c55e'],
    crosshair: {
      xField: { visible: true, line: { type: 'line' } },
    },
  };

  return <VChart spec={spec} option={{ mode: 'desktop-browser' }} />;
};

export default AvailabilityCacheChart;
