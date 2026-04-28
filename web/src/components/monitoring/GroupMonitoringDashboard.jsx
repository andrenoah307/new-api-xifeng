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

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { Button, Input, Select, Skeleton, Typography } from '@douyinfe/semi-ui';
import { Activity, RefreshCw, Search, X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess, isAdmin } from '../../helpers';
import GroupStatusCard from './GroupStatusCard';
import GroupDetailPanel from './GroupDetailPanel';

const { Text } = Typography;

const POLL_INTERVAL_MS = 60 * 1000;
const SORT_KEY = 'monitoring-sort-mode';

const SORT_OPTIONS = [
  { value: 'status', labelKey: '按状态' },
  { value: 'name', labelKey: '按名称' },
  { value: 'availability', labelKey: '按可用率' },
];

function compareGroups(a, b, mode) {
  const aOnline = a.is_online ?? a.online_channels > 0;
  const bOnline = b.is_online ?? b.online_channels > 0;
  switch (mode) {
    case 'name':
      return (a.group_name || '').localeCompare(b.group_name || '');
    case 'availability': {
      const ar = a.availability_rate ?? -1;
      const br = b.availability_rate ?? -1;
      return ar - br;
    }
    case 'status':
    default:
      if (aOnline !== bOnline) return aOnline ? 1 : -1;
      return (a.availability_rate ?? 100) - (b.availability_rate ?? 100);
  }
}

function avgAvailability(groups) {
  const valid = groups
    .map((g) => g.availability_rate)
    .filter((r) => r != null && r >= 0);
  if (valid.length === 0) return null;
  return valid.reduce((s, v) => s + v, 0) / valid.length;
}

function rateAccent(rate) {
  if (rate == null || rate < 0) return 'var(--semi-color-text-2)';
  if (rate >= 99) return 'var(--semi-color-success)';
  if (rate >= 95) return 'var(--semi-color-success-light-active)';
  if (rate >= 80) return 'var(--semi-color-warning)';
  return 'var(--semi-color-danger)';
}

const GroupMonitoringDashboard = () => {
  const { t } = useTranslation();
  const [groups, setGroups] = useState([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [lastUpdated, setLastUpdated] = useState(null);
  const [selectedGroup, setSelectedGroup] = useState(null);
  const [detailVisible, setDetailVisible] = useState(false);
  const [keyword, setKeyword] = useState('');
  const [sortMode, setSortMode] = useState(() => {
    try {
      return localStorage.getItem(SORT_KEY) || 'status';
    } catch {
      return 'status';
    }
  });
  const [countdown, setCountdown] = useState(POLL_INTERVAL_MS);
  const pollTimerRef = useRef(null);
  const tickRef = useRef(null);
  const initialLoad = useRef(true);

  const admin = isAdmin();

  const fetchGroups = useCallback(
    async (includeHistory) => {
      try {
        const prefix = admin ? 'admin' : 'public';
        const res = await API.get(`/api/monitoring/${prefix}/groups`);
        if (res.data.success) {
          let groupData = res.data.data || [];

          if (includeHistory && groupData.length > 0) {
            const historyPromises = groupData.map(async (g) => {
              try {
                const hRes = await API.get(
                  `/api/monitoring/${prefix}/groups/${encodeURIComponent(g.group_name)}/history`,
                );
                if (hRes.data.success) {
                  return {
                    ...g,
                    history: hRes.data.data || [],
                    aggregation_interval_minutes:
                      hRes.data.aggregation_interval_minutes || 5,
                  };
                }
              } catch {
                // ignore per-group history failure
              }
              return g;
            });
            groupData = await Promise.all(historyPromises);
          }

          setGroups((prev) => {
            if (!includeHistory && prev.length > 0) {
              return groupData.map((g) => {
                const old = prev.find((p) => p.group_name === g.group_name);
                if (old) {
                  return {
                    ...g,
                    history: old.history,
                    aggregation_interval_minutes:
                      old.aggregation_interval_minutes,
                  };
                }
                return g;
              });
            }
            return groupData;
          });
          setLastUpdated(Date.now());
        } else {
          showError(res.data.message || t('获取监控数据失败'));
        }
      } catch {
        showError(t('获取监控数据失败'));
      }
    },
    [admin, t],
  );

  useEffect(() => {
    const init = async () => {
      setLoading(true);
      await fetchGroups(true);
      setLoading(false);
      initialLoad.current = false;
      setCountdown(POLL_INTERVAL_MS);
    };
    init();
  }, [fetchGroups]);

  useEffect(() => {
    pollTimerRef.current = setInterval(() => {
      if (!initialLoad.current) {
        fetchGroups(false);
        setCountdown(POLL_INTERVAL_MS);
      }
    }, POLL_INTERVAL_MS);
    return () => clearInterval(pollTimerRef.current);
  }, [fetchGroups]);

  useEffect(() => {
    tickRef.current = setInterval(() => {
      setCountdown((c) => Math.max(0, c - 1000));
    }, 1000);
    return () => clearInterval(tickRef.current);
  }, []);

  const handleRefresh = async () => {
    setRefreshing(true);
    try {
      if (admin) {
        const res = await API.post('/api/monitoring/admin/refresh');
        if (res.data.success) {
          showSuccess(t('刷新成功'));
        }
      }
      await fetchGroups(true);
      setCountdown(POLL_INTERVAL_MS);
    } catch {
      showError(t('刷新失败'));
    } finally {
      setRefreshing(false);
    }
  };

  const handleSort = (val) => {
    setSortMode(val);
    try {
      localStorage.setItem(SORT_KEY, val);
    } catch {
      /* noop */
    }
  };

  const handleCardClick = (group) => {
    if (!admin) return;
    setSelectedGroup(group);
    setDetailVisible(true);
  };

  const visible = useMemo(() => {
    const kw = keyword.trim().toLowerCase();
    const filtered = kw
      ? groups.filter((g) =>
          (g.group_name || '').toLowerCase().includes(kw),
        )
      : groups;
    return [...filtered].sort((a, b) => compareGroups(a, b, sortMode));
  }, [groups, keyword, sortMode]);

  const onlineCount = groups.filter(
    (g) => g.is_online ?? g.online_channels > 0,
  ).length;
  const offlineCount = groups.length - onlineCount;
  const avgAvail = avgAvailability(groups);

  const countdownLabel = `${Math.floor(countdown / 60000)}:${String(
    Math.floor((countdown % 60000) / 1000),
  ).padStart(2, '0')}`;

  return (
    <div className='mt-[60px] px-4 sm:px-8 lg:px-10 pb-12'>
      <div className='mx-auto w-full max-w-[1440px]'>
        {/* Slim title bar */}
        <div className='flex flex-wrap items-center justify-between gap-4 py-6 sm:py-8'>
          <div className='flex flex-wrap items-baseline gap-x-4 gap-y-2'>
            <h1 className='m-0 text-xl font-semibold tracking-tight text-semi-color-text-0'>
              {t('分组监控')}
            </h1>
            {!loading && groups.length > 0 && (
              <div className='flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-semi-color-text-2'>
                <span className='inline-flex items-center gap-1.5'>
                  <span className='inline-block h-1.5 w-1.5 rounded-full bg-semi-color-success' />
                  <span className='font-mono text-semi-color-text-1'>
                    {onlineCount}
                  </span>
                  <span>{t('在线')}</span>
                </span>
                <span className='inline-flex items-center gap-1.5'>
                  <span
                    className='inline-block h-1.5 w-1.5 rounded-full'
                    style={{
                      background:
                        offlineCount > 0
                          ? 'var(--semi-color-danger)'
                          : 'var(--semi-color-fill-2)',
                    }}
                  />
                  <span className='font-mono text-semi-color-text-1'>
                    {offlineCount}
                  </span>
                  <span>{t('离线')}</span>
                </span>
                {avgAvail != null && (
                  <span className='inline-flex items-baseline gap-1.5'>
                    <span>{t('平均可用率')}</span>
                    <span
                      className='font-mono'
                      style={{ color: rateAccent(avgAvail) }}
                    >
                      {avgAvail.toFixed(1)}%
                    </span>
                  </span>
                )}
              </div>
            )}
          </div>

          <div className='flex flex-wrap items-center gap-2'>
            <Input
              prefix={<Search size={14} />}
              placeholder={t('搜索分组')}
              value={keyword}
              onChange={setKeyword}
              showClear
              size='default'
              style={{ width: 200 }}
            />
            <Select
              value={sortMode}
              onChange={handleSort}
              size='default'
              style={{ width: 130 }}
            >
              {SORT_OPTIONS.map((opt) => (
                <Select.Option key={opt.value} value={opt.value}>
                  {t(opt.labelKey)}
                </Select.Option>
              ))}
            </Select>
            <Button
              icon={<RefreshCw size={14} />}
              size='default'
              theme='borderless'
              type='tertiary'
              loading={refreshing}
              onClick={admin ? handleRefresh : undefined}
              disabled={!admin}
              title={
                admin
                  ? t('立即刷新 · 下次自动刷新 {{c}}', { c: countdownLabel })
                  : t('下次自动刷新 {{c}}', { c: countdownLabel })
              }
            >
              <span className='font-mono text-[11px] text-semi-color-text-3'>
                {countdownLabel}
              </span>
            </Button>
          </div>
        </div>

        {/* Cards grid */}
        {loading ? (
          <div className='grid grid-cols-1 gap-4 sm:gap-5 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4'>
            {[1, 2, 3, 4, 5, 6].map((i) => (
              <div
                key={i}
                className='rounded-2xl border border-semi-color-border bg-semi-color-bg-1 p-5'
              >
                <Skeleton.Title style={{ width: '50%', marginBottom: 16 }} />
                <Skeleton.Paragraph rows={2} />
                <Skeleton.Image
                  style={{ width: '100%', height: 24, marginTop: 16 }}
                />
              </div>
            ))}
          </div>
        ) : groups.length === 0 ? (
          <EmptyState
            icon={<Activity size={32} className='opacity-40' />}
            title={t('暂无监控分组')}
            desc={t('请在系统设置 — 分组监控中配置')}
          />
        ) : visible.length === 0 ? (
          <EmptyState
            icon={<X size={32} className='opacity-40' />}
            title={t('未找到匹配 "{{kw}}" 的分组', { kw: keyword })}
          />
        ) : (
          <div className='grid grid-cols-1 gap-4 sm:gap-5 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4'>
            {visible.map((g) => (
              <GroupStatusCard
                key={g.group_name}
                group={g}
                onClick={admin ? handleCardClick : undefined}
              />
            ))}
          </div>
        )}

        <GroupDetailPanel
          visible={detailVisible}
          group={selectedGroup}
          onClose={() => {
            setDetailVisible(false);
            setSelectedGroup(null);
          }}
        />
      </div>
    </div>
  );
};

const EmptyState = ({ icon, title, desc }) => (
  <div className='flex flex-col items-center justify-center rounded-2xl border border-dashed border-semi-color-border py-24 text-center'>
    <div className='mb-4 text-semi-color-text-2'>{icon}</div>
    <Text strong className='!text-base !text-semi-color-text-1'>
      {title}
    </Text>
    {desc && (
      <Text type='tertiary' size='small' className='!mt-1.5'>
        {desc}
      </Text>
    )}
  </div>
);

export default GroupMonitoringDashboard;
