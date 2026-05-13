import React, { useState, useEffect, useRef, useMemo, useCallback } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Tabs, TabPane, Spin, Empty, Card } from '@douyinfe/semi-ui';
import {
  BarChart3,
  Trophy,
  PieChart,
  TrendingUp,
  TrendingDown,
  ArrowUpRight,
  ArrowDownRight,
} from 'lucide-react';
import { VChart } from '@visactor/react-vchart';
import { initVChartSemiTheme } from '@visactor/vchart-semi-theme';
import { API } from '../../helpers';

const VCHART_OPTION = { mode: 'desktop-browser' };
const CACHE_TTL = 5 * 60 * 1000;

const VALID_PERIODS = ['today', 'week', 'month', 'year', 'all'];

const PERIOD_LABELS = {
  today: '今日',
  week: '本周',
  month: '本月',
  year: '本年',
  all: '全部',
};

function formatTokens(value) {
  if (!Number.isFinite(value) || value <= 0) return '0';
  if (value >= 1e12) return `${(value / 1e12).toFixed(2)}T`;
  if (value >= 1e9) return `${(value / 1e9).toFixed(value >= 1e10 ? 1 : 2)}B`;
  if (value >= 1e6) return `${(value / 1e6).toFixed(value >= 1e7 ? 1 : 2)}M`;
  if (value >= 1e3) return `${(value / 1e3).toFixed(value >= 1e4 ? 0 : 1)}K`;
  return value.toLocaleString();
}

function formatShare(share) {
  if (!Number.isFinite(share) || share <= 0) return '0%';
  if (share < 0.001) return '<0.1%';
  return `${(share * 100).toFixed(share < 0.01 ? 2 : 1)}%`;
}

const VENDOR_COLOURS = {
  OpenAI: '#10a37f',
  Anthropic: '#d97757',
  Google: '#4285f4',
  DeepSeek: '#7c5cff',
  Alibaba: '#ff9900',
  xAI: '#1f2937',
  Meta: '#1877f2',
  Moonshot: '#ec4899',
  Zhipu: '#06b6d4',
  Mistral: '#ff7000',
  ByteDance: '#3b82f6',
  Tencent: '#22c55e',
  MiniMax: '#a855f7',
  Cohere: '#fb923c',
  Baidu: '#ef4444',
  Others: '#94a3b8',
};

const FALLBACK_PALETTE = [
  '#0ea5e9', '#22c55e', '#a855f7', '#f97316', '#14b8a6',
  '#eab308', '#ec4899', '#84cc16', '#6366f1', '#10b981',
  '#f43f5e', '#0891b2', '#94a3b8',
];

function buildVendorColourMap(names) {
  const result = {};
  let idx = 0;
  for (const name of names) {
    if (VENDOR_COLOURS[name]) {
      result[name] = VENDOR_COLOURS[name];
    } else {
      result[name] = FALLBACK_PALETTE[idx % FALLBACK_PALETTE.length];
      idx++;
    }
  }
  return result;
}

const RankingsPage = () => {
  const { t } = useTranslation();
  const [searchParams, setSearchParams] = useSearchParams();
  const [snapshot, setSnapshot] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const cacheRef = useRef({});

  const period = VALID_PERIODS.includes(searchParams.get('period'))
    ? searchParams.get('period')
    : 'week';

  useEffect(() => {
    initVChartSemiTheme({ isWatchingThemeSwitch: true });
  }, []);

  const fetchRankings = useCallback(async (p) => {
    const cached = cacheRef.current[p];
    if (cached && Date.now() - cached.ts < CACHE_TTL) {
      setSnapshot(cached.data);
      setLoading(false);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const res = await API.get('/api/rankings', { params: { period: p } });
      if (res.data.success) {
        setSnapshot(res.data.data);
        cacheRef.current[p] = { data: res.data.data, ts: Date.now() };
      } else {
        setError(res.data.message || t('加载失败'));
      }
    } catch (e) {
      setError(e.message || t('加载失败'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    fetchRankings(period);
  }, [period, fetchRankings]);

  const handlePeriodChange = useCallback((key) => {
    setSearchParams({ period: key });
  }, [setSearchParams]);

  return (
    <div className="px-4 py-8 sm:px-8 lg:px-10 max-w-[1280px] mx-auto">
      <div className="mb-8">
        <h1 className="text-2xl font-bold mb-4">{t('排行榜')}</h1>
        <Tabs type="button" activeKey={period} onChange={handlePeriodChange}>
          {VALID_PERIODS.map((p) => (
            <TabPane tab={t(PERIOD_LABELS[p])} itemKey={p} key={p} />
          ))}
        </Tabs>
      </div>

      {loading ? (
        <div className="flex items-center justify-center py-24">
          <Spin size="large" />
        </div>
      ) : error ? (
        <div className="flex items-center justify-center py-24">
          <Empty
            title={t('加载失败')}
            description={error}
          />
        </div>
      ) : !snapshot ? (
        <div className="flex items-center justify-center py-24">
          <Empty title={t('暂无数据')} />
        </div>
      ) : (
        <div className="space-y-6">
          <ModelsSection
            history={snapshot.models_history}
            rows={snapshot.models}
            t={t}
          />
          <MarketShareSection
            history={snapshot.vendor_share_history}
            rows={snapshot.vendors}
            t={t}
          />
          <PulseSection
            movers={snapshot.top_movers}
            droppers={snapshot.top_droppers}
            t={t}
          />
        </div>
      )}
    </div>
  );
};

const TOOLTIP_MAX_ROWS = 10;

function ModelsSection({ history, rows, t }) {
  const orderedPoints = useMemo(() => {
    if (!history?.points?.length) return [];
    const order = new Map(
      (history.models || []).map((m, idx) => [m.name, idx])
    );
    return [...history.points].sort((a, b) => {
      const tsCmp = (a.ts || '').localeCompare(b.ts || '');
      if (tsCmp !== 0) return tsCmp;
      return (order.get(a.model) ?? 999) - (order.get(b.model) ?? 999);
    });
  }, [history]);

  const totalTokens = useMemo(
    () => (rows || []).reduce((s, r) => s + (r.total_tokens || 0), 0),
    [rows]
  );

  const spec = useMemo(() => {
    if (orderedPoints.length === 0) return null;
    return {
      type: 'bar',
      data: [{ id: 'models-history', values: orderedPoints }],
      xField: 'label',
      yField: 'tokens',
      seriesField: 'model',
      stack: true,
      bar: { style: { cornerRadius: 2 } },
      legends: { visible: false },
      axes: [
        {
          orient: 'bottom',
          label: { style: { fontSize: 10 }, autoHide: true, autoLimit: true },
          tick: { visible: false },
        },
        {
          orient: 'left',
          label: { formatMethod: (val) => formatTokens(Number(val)), style: { fontSize: 10 } },
          grid: { visible: true, style: { lineDash: [3, 3] } },
        },
      ],
      tooltip: {
        dimension: {
          title: { value: (datum) => String(datum?.label ?? '') },
          content: [
            {
              key: (datum) => String(datum?.model ?? ''),
              value: (datum) => Number(datum?.tokens) || 0,
            },
          ],
          updateContent: (array) => {
            array.sort((a, b) => Number(b.value) - Number(a.value));
            const sum = array.reduce((s, x) => s + (Number(x.value) || 0), 0);
            const visible = array.slice(0, TOOLTIP_MAX_ROWS);
            const overflow = array.slice(TOOLTIP_MAX_ROWS);
            const result = visible.map((item) => ({
              key: item.key,
              value: formatTokens(Number(item.value) || 0),
            }));
            if (overflow.length > 0) {
              const otherSum = overflow.reduce((s, item) => s + (Number(item.value) || 0), 0);
              result.push({ key: `+${overflow.length}`, value: formatTokens(otherSum) });
            }
            result.unshift({ key: t('合计'), value: formatTokens(sum) });
            return result;
          },
        },
      },
      animationAppear: { duration: 500 },
      background: 'transparent',
    };
  }, [orderedPoints, t]);

  return (
    <Card className="!rounded-2xl overflow-hidden">
      <div className="flex items-start justify-between gap-4 mb-4">
        <div>
          <h2 className="text-base font-semibold flex items-center gap-2">
            <BarChart3 size={16} className="text-blue-500" />
            {t('热门模型')}
          </h2>
        </div>
        <div className="text-right">
          <div className="font-mono text-2xl font-semibold tabular-nums">
            {formatTokens(totalTokens)}
          </div>
          <div className="text-xs text-gray-500 uppercase tracking-wider">tokens</div>
        </div>
      </div>
      <div className="h-60 sm:h-72">
        {spec ? (
          <VChart spec={spec} option={VCHART_OPTION} skipFunctionDiff />
        ) : (
          <div className="flex h-full items-center justify-center text-xs text-gray-400">
            {t('暂无数据')}
          </div>
        )}
      </div>

      <div className="border-t mt-4 pt-4">
        <h3 className="text-sm font-semibold flex items-center gap-2 mb-3">
          <Trophy size={14} className="text-amber-500" />
          {t('LLM 排行榜')}
        </h3>
        {(!rows || rows.length === 0) ? (
          <div className="text-center text-sm text-gray-400 py-8">{t('暂无数据')}</div>
        ) : (
          <ModelLeaderboard rows={rows} />
        )}
      </div>
    </Card>
  );
}

function ModelLeaderboard({ rows }) {
  const half = Math.ceil(rows.length / 2);
  const left = rows.slice(0, half);
  const right = rows.slice(half);
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 gap-x-8">
      <ModelList rows={left} />
      {right.length > 0 && <ModelList rows={right} />}
    </div>
  );
}

function ModelList({ rows }) {
  return (
    <ul>
      {rows.map((row) => {
        const delta = row.previous_rank != null ? row.previous_rank - row.rank : null;
        return (
          <li key={row.model_name} className="flex items-center gap-3 py-2">
            <span className="w-6 text-right font-mono text-xs text-gray-500 tabular-nums shrink-0">
              {row.rank}.
            </span>
            <div className="min-w-0 flex-1">
              <span className="block truncate font-mono text-xs font-medium">
                {row.model_name}
              </span>
              <span className="text-[11px] text-gray-500 truncate block">
                {row.vendor}
              </span>
            </div>
            <div className="text-right shrink-0">
              <div className="font-mono text-sm font-semibold tabular-nums">
                {formatTokens(row.total_tokens)}
              </div>
              <div className="font-mono text-[11px] tabular-nums">
                {delta != null && delta !== 0 && (
                  <span className={delta > 0 ? 'text-green-600' : 'text-red-500'}>
                    {delta > 0 ? (
                      <><ArrowUpRight size={10} className="inline" />{delta}</>
                    ) : (
                      <><ArrowDownRight size={10} className="inline" />{Math.abs(delta)}</>
                    )}
                  </span>
                )}
                {row.growth_pct != null && (
                  <span className={`ml-1.5 ${row.growth_pct >= 0 ? 'text-green-600' : 'text-red-500'}`}>
                    {row.growth_pct >= 0 ? '+' : ''}{row.growth_pct.toFixed(1)}%
                  </span>
                )}
              </div>
            </div>
          </li>
        );
      })}
    </ul>
  );
}

function MarketShareSection({ history, rows, t }) {
  const colourMap = useMemo(
    () => buildVendorColourMap((history?.vendors || []).map((v) => v.name)),
    [history]
  );

  const orderedPoints = useMemo(() => {
    if (!history?.points?.length) return [];
    const order = new Map(
      (history.vendors || []).map((v, idx) => [v.name, idx])
    );
    return [...history.points].sort((a, b) => {
      const tsCmp = (a.ts || '').localeCompare(b.ts || '');
      if (tsCmp !== 0) return tsCmp;
      return (order.get(a.vendor) ?? 999) - (order.get(b.vendor) ?? 999);
    });
  }, [history]);

  const spec = useMemo(() => {
    if (orderedPoints.length === 0) return null;
    return {
      type: 'bar',
      data: [{ id: 'vendor-share', values: orderedPoints }],
      xField: 'label',
      yField: 'share',
      seriesField: 'vendor',
      stack: true,
      paddingInner: 0.12,
      legends: { visible: false },
      bar: { style: { cornerRadius: 2 } },
      color: { specified: colourMap },
      axes: [
        {
          orient: 'bottom',
          label: { style: { fontSize: 10 }, autoHide: true, autoLimit: true },
          tick: { visible: false },
        },
        {
          orient: 'left',
          min: 0,
          max: 1,
          label: {
            formatMethod: (val) => `${Math.round(Number(val) * 100)}%`,
            style: { fontSize: 10 },
          },
          grid: { visible: true, style: { lineDash: [3, 3] } },
        },
      ],
      tooltip: {
        dimension: {
          title: { value: (datum) => String(datum?.label ?? '') },
          content: [
            {
              key: (datum) => String(datum?.vendor ?? ''),
              value: (datum) => Number(datum?.share) || 0,
            },
          ],
          updateContent: (array) =>
            array
              .filter((item) => Number(item.value) > 0.001)
              .sort((a, b) => Number(b.value) - Number(a.value))
              .map((item) => ({
                key: item.key,
                value: `${(Number(item.value) * 100).toFixed(1)}%`,
              })),
        },
      },
      animationAppear: { duration: 500 },
      background: 'transparent',
    };
  }, [colourMap, orderedPoints]);

  const visible = (rows || []).slice(0, 12);
  const half = Math.ceil(visible.length / 2);
  const left = visible.slice(0, half);
  const right = visible.slice(half);

  return (
    <Card className="!rounded-2xl overflow-hidden">
      <div className="mb-4">
        <h2 className="text-base font-semibold flex items-center gap-2">
          <PieChart size={16} className="text-blue-500" />
          {t('市场份额')}
        </h2>
      </div>
      <div className="h-60 sm:h-72">
        {spec ? (
          <VChart spec={spec} option={VCHART_OPTION} skipFunctionDiff />
        ) : (
          <div className="flex h-full items-center justify-center text-xs text-gray-400">
            {t('暂无数据')}
          </div>
        )}
      </div>

      <div className="border-t mt-4 pt-4">
        <h3 className="text-sm font-semibold mb-3">{t('按模型作者')}</h3>
        {visible.length === 0 ? (
          <div className="text-center text-sm text-gray-400 py-8">{t('暂无数据')}</div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-x-8">
            <VendorList rows={left} colourMap={colourMap} />
            {right.length > 0 && <VendorList rows={right} colourMap={colourMap} />}
          </div>
        )}
      </div>
    </Card>
  );
}

function VendorList({ rows, colourMap }) {
  return (
    <ul>
      {rows.map((vendor) => (
        <li key={vendor.vendor} className="flex items-center gap-3 py-2.5">
          <span className="w-6 text-right font-mono text-xs text-gray-500 tabular-nums shrink-0">
            {vendor.rank}.
          </span>
          <span
            className="w-2.5 h-2.5 rounded-full shrink-0"
            style={{ backgroundColor: colourMap[vendor.vendor] || '#94a3b8' }}
          />
          <span className="min-w-0 flex-1 truncate text-sm font-medium">
            {vendor.vendor}
          </span>
          <div className="text-right shrink-0">
            <div className="font-mono text-sm font-semibold tabular-nums">
              {formatTokens(vendor.total_tokens)}
            </div>
            <div className="font-mono text-[11px] text-gray-500 tabular-nums">
              {formatShare(vendor.share)}
            </div>
          </div>
        </li>
      ))}
    </ul>
  );
}

function PulseSection({ movers, droppers, t }) {
  return (
    <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
      <PulseCard
        title={t('上升趋势')}
        icon={<TrendingUp size={16} className="text-green-500" />}
      >
        {(!movers || movers.length === 0) ? (
          <div className="text-center text-xs text-gray-400 py-6">{t('暂无数据')}</div>
        ) : (
          <ul>
            {movers.map((row) => (
              <MoverRow key={row.model_name} row={row} intent="up" />
            ))}
          </ul>
        )}
      </PulseCard>

      <PulseCard
        title={t('下降趋势')}
        icon={<TrendingDown size={16} className="text-red-500" />}
      >
        {(!droppers || droppers.length === 0) ? (
          <div className="text-center text-xs text-gray-400 py-6">{t('暂无数据')}</div>
        ) : (
          <ul>
            {droppers.map((row) => (
              <MoverRow key={row.model_name} row={row} intent="down" />
            ))}
          </ul>
        )}
      </PulseCard>
    </div>
  );
}

function PulseCard({ title, icon, children }) {
  return (
    <Card className="!rounded-2xl overflow-hidden">
      <div className="flex items-center gap-2 mb-3">
        {icon}
        <h3 className="text-sm font-semibold">{title}</h3>
      </div>
      {children}
    </Card>
  );
}

function MoverRow({ row, intent }) {
  return (
    <li className="flex items-center gap-3 py-2">
      <div className="min-w-0 flex-1">
        <span className="block truncate font-mono text-xs font-medium">
          {row.model_name}
        </span>
        <span className="text-[11px] text-gray-500">
          #{row.current_rank} · {row.vendor}
        </span>
      </div>
      <span
        className={`inline-flex items-center gap-0.5 font-mono text-xs font-semibold tabular-nums shrink-0 ${
          intent === 'up' ? 'text-green-600' : 'text-red-500'
        }`}
      >
        {intent === 'up' ? <ArrowUpRight size={12} /> : <ArrowDownRight size={12} />}
        {Math.abs(row.rank_delta)}
      </span>
    </li>
  );
}

export default RankingsPage;
