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
import {
  Button,
  Input,
  Select,
  Skeleton,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { Activity, Clock, RefreshCw, Search, X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess, isAdmin } from '../../helpers';
import GroupStatusCard from './GroupStatusCard';
import GroupDetailPanel from './GroupDetailPanel';

const { Text, Title } = Typography;

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
      return ar - br; // worst first
    }
    case 'status':
    default:
      // offline first, then by availability ascending (worst surfaces first)
      if (aOnline !== bOnline) return aOnline ? 1 : -1;
      return (a.availability_rate ?? 100) - (b.availability_rate ?? 100);
  }
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
                    aggregation_interval_minutes: old.aggregation_interval_minutes,
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

  // Initial load with history
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

  // Polling
  useEffect(() => {
    pollTimerRef.current = setInterval(() => {
      if (!initialLoad.current) {
        fetchGroups(false);
        setCountdown(POLL_INTERVAL_MS);
      }
    }, POLL_INTERVAL_MS);
    return () => clearInterval(pollTimerRef.current);
  }, [fetchGroups]);

  // 1Hz countdown tick
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
  const countdownLabel = `${Math.floor(countdown / 60000)}:${String(
    Math.floor((countdown % 60000) / 1000),
  ).padStart(2, '0')}`;

  return (
    <div className='space-y-4 p-4 sm:p-5'>
      {/* Toolbar */}
      <div className='flex flex-wrap items-center gap-3'>
        <div className='flex items-center gap-3'>
          <Title heading={5} className='!m-0 flex items-center gap-2'>
            <Activity
              size={18}
              className='text-semi-color-primary'
              aria-hidden
            />
            {t('分组监控')}
          </Title>
          <Tag color='green' size='small' shape='circle'>
            {t('在线')} {onlineCount}
          </Tag>
          <Tag color='red' size='small' shape='circle'>
            {t('离线')} {offlineCount}
          </Tag>
          <Text type='tertiary' size='small'>
            {t('共 {{n}} 个', { n: groups.length })}
          </Text>
        </div>
        <div className='ml-auto flex flex-wrap items-center gap-2'>
          <Input
            prefix={<Search size={14} />}
            placeholder={t('搜索分组名')}
            value={keyword}
            onChange={setKeyword}
            showClear
            size='small'
            style={{ width: 180 }}
          />
          <Select
            value={sortMode}
            onChange={handleSort}
            size='small'
            style={{ width: 120 }}
          >
            {SORT_OPTIONS.map((opt) => (
              <Select.Option key={opt.value} value={opt.value}>
                {t(opt.labelKey)}
              </Select.Option>
            ))}
          </Select>
          <div className='flex items-center gap-1.5 rounded-md bg-semi-color-fill-0 px-2 py-1 text-[11px] text-semi-color-text-2'>
            <Clock size={12} />
            <span className='font-mono'>{countdownLabel}</span>
          </div>
          {admin && (
            <Button
              icon={<RefreshCw size={14} />}
              size='small'
              loading={refreshing}
              onClick={handleRefresh}
            >
              {t('刷新')}
            </Button>
          )}
        </div>
      </div>

      {/* Last updated row */}
      {lastUpdated && (
        <div className='-mt-2 text-[11px] text-semi-color-text-3'>
          {t('更新于')}{' '}
          {new Date(lastUpdated).toLocaleTimeString([], {
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit',
            hour12: false,
          })}
        </div>
      )}

      {/* Cards */}
      {loading ? (
        <div className='grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4'>
          {[1, 2, 3, 4, 5, 6].map((i) => (
            <div
              key={i}
              className='rounded-2xl border border-semi-color-border bg-semi-color-bg-1 p-4'
            >
              <Skeleton.Title style={{ width: '50%', marginBottom: 12 }} />
              <Skeleton.Paragraph rows={2} />
              <Skeleton.Image style={{ width: '100%', height: 22, marginTop: 12 }} />
            </div>
          ))}
        </div>
      ) : groups.length === 0 ? (
        <div className='flex flex-col items-center justify-center rounded-2xl border border-dashed border-semi-color-border py-16 text-semi-color-text-2'>
          <Activity size={28} className='mb-3 opacity-40' />
          <Text type='tertiary' size='normal'>
            {t('暂无监控分组，请在设置中配置')}
          </Text>
        </div>
      ) : visible.length === 0 ? (
        <div className='flex flex-col items-center justify-center rounded-2xl border border-dashed border-semi-color-border py-16 text-semi-color-text-2'>
          <X size={28} className='mb-3 opacity-40' />
          <Text type='tertiary' size='normal'>
            {t('未找到匹配 "{{kw}}" 的分组', { kw: keyword })}
          </Text>
        </div>
      ) : (
        <div className='grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4'>
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
  );
};

export default GroupMonitoringDashboard;
