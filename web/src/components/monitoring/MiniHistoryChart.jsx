import React, { useMemo } from 'react';
import { VChart } from '@visactor/react-vchart';

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

const MiniHistoryChart = ({ history, intervalMinutes }) => {
  const chartData = useMemo(
    () => alignAndFillHistory(history, intervalMinutes),
    [history, intervalMinutes]
  );

  const timeLabels = useMemo(() => {
    if (!chartData || chartData.length === 0) return { first: '', last: '' };
    const avails = chartData.filter((d) => d.type === 'availability');
    if (avails.length === 0) return { first: '', last: '' };
    return {
      first: avails[0].time,
      last: avails[avails.length - 1].time,
    };
  }, [chartData]);

  if (!chartData || chartData.length === 0) {
    return (
      <div
        style={{
          height: 80,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: 'var(--semi-color-text-2)',
          fontSize: 12,
        }}
      />
    );
  }

  const spec = {
    type: 'line',
    data: [{ id: 'mini', values: chartData }],
    xField: 'time',
    yField: 'value',
    seriesField: 'type',
    width: 'auto',
    height: 80,
    padding: { top: 4, bottom: 0, left: 0, right: 0 },
    animation: false,
    line: {
      style: {
        lineWidth: 1.5,
        curveType: 'monotone',
      },
    },
    point: { visible: false },
    axes: [
      { orient: 'bottom', visible: false },
      { orient: 'left', visible: false },
    ],
    legends: { visible: false },
    tooltip: { visible: false },
    color: ['#3b82f6', '#22c55e'],
    crosshair: { xField: { visible: false }, yField: { visible: false } },
  };

  return (
    <div>
      <VChart spec={spec} option={{ mode: 'desktop-browser' }} />
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          fontSize: 10,
          color: 'var(--semi-color-text-2)',
          marginTop: 2,
          padding: '0 2px',
        }}
      >
        <span>{timeLabels.first}</span>
        <span>{timeLabels.last}</span>
      </div>
    </div>
  );
};

export default MiniHistoryChart;
