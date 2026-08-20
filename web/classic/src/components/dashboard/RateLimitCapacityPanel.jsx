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
  useContext,
  useEffect,
  useRef,
  useState,
} from 'react';
import { Banner, Button, Card, Spin, Tag, Typography } from '@douyinfe/semi-ui';
import { ChevronDown, ChevronUp, Gauge, RefreshCw } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API } from '../../helpers';
import {
  normalizePersonalRPMItems,
  personalRPMDisplayState,
} from '../../helpers/personal-rpm';
import {
  isAnyRateLimitCapacityExpanded,
  shouldRequestRateLimitCapacity,
} from '../../helpers/rate-limit-capacity';
import { UserContext } from '../../context/User';

const { Text } = Typography;
// Dedupes non-interactive fetches triggered close together (mount, identity
// change, hover). Explicit refreshes bypass this window.
const STALE_TIME = 5000;
const RETRY_COOLDOWN = 3000;

function formatCount(value) {
  return Number.isFinite(value) ? value.toLocaleString() : '0';
}

function formatPercent(value) {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return null;
  }
  const percent = value * 100;
  if (percent >= 100) {
    return `${Math.ceil(percent * 10) / 10}%`;
  }
  return `${percent.toFixed(1)}%`;
}

function formatObservedAt(value) {
  if (!value) return '--';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '--' : date.toLocaleString();
}

function metricAvailable(metric) {
  return Boolean(
    metric &&
    metric.available &&
    typeof metric.current === 'number' &&
    Number.isFinite(metric.current),
  );
}

function CapacityBar({ metric }) {
  if (
    !metric ||
    !metricAvailable(metric) ||
    metric.utilization === null ||
    metric.utilization === undefined
  ) {
    return null;
  }

  const width = Math.min(100, Math.max(0, metric.utilization * 100));
  const color = metric.over_limit
    ? 'var(--semi-color-danger)'
    : metric.utilization >= 0.8
      ? 'var(--semi-color-warning)'
      : 'var(--semi-color-primary)';

  return (
    <div
      style={{
        height: 6,
        width: '100%',
        borderRadius: 999,
        background: 'var(--semi-color-fill-2)',
        overflow: 'hidden',
      }}
      aria-hidden='true'
    >
      <div
        style={{
          width: `${width}%`,
          height: '100%',
          borderRadius: 999,
          background: color,
        }}
      />
    </div>
  );
}

function CapacityRow({ label, metric, badge, t }) {
  const available = metricAvailable(metric);
  const percent = formatPercent(metric?.utilization);
  let value = t('暂时不可用');
  if (available) {
    value = `${formatCount(metric.current)} / ${formatCount(metric.limit)}`;
  }

  const valueColor = metric?.over_limit
    ? 'var(--semi-color-danger)'
    : available && metric?.utilization >= 0.8
      ? 'var(--semi-color-warning)'
      : undefined;

  return (
    <div className='rounded-lg border border-gray-200 p-3'>
      <div className='flex min-w-0 items-center gap-2'>
        <div className='min-w-0 truncate text-sm'>
          <Text strong ellipsis={{ showTooltip: true }}>
            {label}
          </Text>
        </div>
        {badge && <Tag color='blue'>{badge}</Tag>}
      </div>
      <div className='mt-2 flex min-w-0 items-baseline gap-2'>
        <div
          className='min-w-0 truncate font-mono text-sm font-semibold tabular-nums'
          style={{ color: valueColor }}
        >
          {value}
        </div>
        {available && percent && (
          <div className='shrink-0 text-xs font-normal text-gray-500'>
            {percent}
          </div>
        )}
      </div>
      <div className='mt-2'>
        <CapacityBar metric={metric} />
      </div>
    </div>
  );
}

function CapacitySectionHeader({
  title,
  windowLabel,
  total,
  displayedCount,
  expanded,
  loading,
  label,
  controlsId,
  onIntent,
  onToggle,
  t,
}) {
  return (
    <div className='flex items-center justify-between gap-3 border-b border-gray-200 bg-gray-50 px-4 py-3 sm:px-5'>
      <div className='min-w-0'>
        <Text strong>{title}</Text>
        <div className='mt-1 text-xs text-gray-500'>
          {windowLabel} · {t('全站用户')}
        </div>
      </div>
      {(expanded || total > displayedCount) && (
        <CapacityExpandButton
          expanded={expanded}
          total={total}
          loading={loading}
          label={label}
          controlsId={controlsId}
          onIntent={onIntent}
          onToggle={onToggle}
          t={t}
        />
      )}
    </div>
  );
}

function CapacityGroupCard({
  group,
  index,
  expanded,
  loading,
  onIntent,
  onToggle,
  t,
}) {
  const contentId = `classic-rate-limit-capacity-group-${index}`;
  const items = expanded ? group.items : group.items.slice(0, 3);
  return (
    <article className='rounded-lg border border-gray-200 bg-white p-3'>
      <div className='min-w-0'>
        <div className='min-w-0 truncate text-sm'>
          <Text strong ellipsis={{ showTooltip: true }}>
            {group.group}
          </Text>
        </div>
        <div className='mt-1 text-xs text-gray-500'>
          {t('{{count}} 个模型', { count: group.total })}
        </div>
      </div>
      <div
        id={contentId}
        className='mt-3 grid gap-3'
        style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(13rem, 1fr))' }}
      >
        {items.map((item) => (
          <CapacityRow
            key={`${item.model}:${item.group || ''}`}
            label={item.model}
            metric={item}
            t={t}
          />
        ))}
      </div>
      {(expanded || group.total > items.length) && (
        <div className='mt-2 flex justify-end'>
          <CapacityExpandButton
            expanded={expanded}
            total={group.total}
            loading={loading}
            label='models'
            controlsId={contentId}
            onIntent={onIntent}
            onToggle={onToggle}
            t={t}
          />
        </div>
      )}
    </article>
  );
}

function GroupTotalCapacitySection({
  section,
  expanded,
  loading,
  onIntent,
  onToggle,
  t,
}) {
  const items = section?.items || [];
  if (!section || (section.total === 0 && items.length === 0)) return null;

  const displayedItems = expanded ? items : items.slice(0, 3);
  return (
    <section aria-label={t('分组总额 RPM')}>
      <CapacitySectionHeader
        title={t('分组总额 RPM')}
        windowLabel={t('固定 60 秒窗口')}
        total={section.total}
        displayedCount={displayedItems.length}
        expanded={expanded}
        loading={loading}
        label='groups'
        controlsId='classic-rate-limit-capacity-group-totals'
        onIntent={onIntent}
        onToggle={onToggle}
        t={t}
      />
      <div
        id='classic-rate-limit-capacity-group-totals'
        className='grid gap-3 px-4 py-3 sm:px-5'
        style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(15rem, 1fr))' }}
      >
        {displayedItems.map((item) => {
          const label = item.group
            ? `${item.group} · ${t('全组合计')}`
            : t('全组合计');
          return (
            <CapacityRow
              key={item.group || label}
              label={label}
              metric={item}
              t={t}
            />
          );
        })}
      </div>
    </section>
  );
}

function PersonalSection({ personal, t }) {
  const items = normalizePersonalRPMItems(personal?.items);
  const displayState = personalRPMDisplayState(personal?.status, items);

  return (
    <section aria-label={t('我的模型 RPM')}>
      <div className='border-b border-gray-200 bg-gray-50 px-4 py-3 sm:px-5'>
        <Text strong>{t('我的模型 RPM')}</Text>
        <div className='mt-1 text-xs text-gray-500'>{t('最近 60 秒')}</div>
      </div>
      {displayState === 'empty' && (
        <div className='px-4 py-4 text-sm text-gray-500 sm:px-5'>
          {t('暂无请求数据统计')}
        </div>
      )}
      {displayState === 'unavailable' && (
        <div className='px-4 py-4 text-sm text-gray-500 sm:px-5'>
          {t('暂时不可用')}
        </div>
      )}
      {displayState === 'available' && (
        <div
          className='grid gap-3 px-4 py-3 sm:px-5'
          style={{
            gridTemplateColumns: 'repeat(auto-fill, minmax(15rem, 1fr))',
          }}
        >
          {items.map((item) => (
            <CapacityRow
              key={item.model || `group:${item.group}`}
              label={item.model || item.group}
              metric={item}
              badge={item.model ? undefined : t('全组合计')}
              t={t}
            />
          ))}
        </div>
      )}
    </section>
  );
}

function CapacityMetadata({ data, refreshing, onRefresh, t }) {
  const instanceOnly =
    data.instance_only ||
    data.backend_scope === 'instance' ||
    Boolean(data.personal?.instance_only);
  return (
    <div className='flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3 text-xs text-gray-500 sm:px-5'>
      <span>
        {t('数据更新：{{time}}', { time: formatObservedAt(data.observed_at) })}
      </span>
      <span>{t('点击刷新获取最新数据')}</span>
      {instanceOnly && (
        <span className='text-amber-600'>{t('仅当前实例')}</span>
      )}
      <Button
        type='button'
        theme='borderless'
        size='small'
        className='ml-auto shrink-0'
        icon={<RefreshCw size={14} />}
        loading={refreshing}
        disabled={refreshing}
        aria-label={t('刷新')}
        onClick={onRefresh}
      >
        {t('刷新')}
      </Button>
    </div>
  );
}

function CapacityExpandButton({
  expanded,
  total,
  loading,
  label,
  controlsId,
  onIntent,
  onToggle,
  t,
}) {
  let icon = <ChevronDown size={14} />;
  let buttonLabel = t('显示全部 {{total}} 个模型', { total });
  if (expanded) {
    icon = <ChevronUp size={14} />;
    buttonLabel = t('收起');
  } else if (loading) {
    icon = <Spin size='small' spinning />;
  }
  if (!expanded && label === 'groups') {
    buttonLabel = t('显示全部 {{total}} 个分组', { total });
  }
  return (
    <Button
      theme='borderless'
      size='small'
      icon={icon}
      aria-expanded={expanded}
      aria-controls={controlsId}
      onMouseEnter={onIntent}
      onFocus={onIntent}
      onTouchStart={onIntent}
      onClick={onToggle}
    >
      {buttonLabel}
    </Button>
  );
}

export default function RateLimitCapacityPanel() {
  const { t } = useTranslation();
  const [userState] = useContext(UserContext);
  const user = userState?.user;
  const hasUser = Boolean(user);
  const identityKey = `${user?.id || 'anonymous'}:${user?.group || ''}`;
  const [globalExpanded, setGlobalExpanded] = useState(false);
  const [groupsExpanded, setGroupsExpanded] = useState(false);
  const [groupTotalsExpanded, setGroupTotalsExpanded] = useState(false);
  const [expandedGroups, setExpandedGroups] = useState({});
  const [topData, setTopData] = useState(null);
  const [allData, setAllData] = useState(null);
  const [topLoading, setTopLoading] = useState(false);
  const [allLoading, setAllLoading] = useState(false);
  const [topError, setTopError] = useState(false);
  const [allError, setAllError] = useState(false);
  const loadedRef = useRef({ top: false, all: false });
  const inFlightRef = useRef({ top: false, all: false });
  const loadedAtRef = useRef({ top: 0, all: 0 });
  const retryAfterRef = useRef({ top: 0, all: 0 });
  const identityRef = useRef(identityKey);
  const initializedIdentityRef = useRef(null);

  const loadCapacity = useCallback(
    async (scope, { force = false } = {}) => {
      if (!hasUser) return;
      const requestIdentity = identityKey;
      const now = Date.now();
      const shouldRequest = shouldRequestRateLimitCapacity({
        force,
        loaded: loadedRef.current[scope],
        loadedAt: loadedAtRef.current[scope],
        now,
        inFlight: inFlightRef.current[scope],
        retryAfterAt: retryAfterRef.current[scope],
        staleTime: STALE_TIME,
      });
      if (!shouldRequest) return;

      inFlightRef.current[scope] = true;
      loadedRef.current[scope] = false;
      if (scope === 'top') {
        setTopLoading(true);
        setTopError(false);
      } else {
        setAllLoading(true);
        setAllError(false);
      }

      try {
        const res = await API.get(`/api/rate_limit/capacity?scope=${scope}`, {
          // This is optional dashboard data. Network failures must fail open
          // without the Classic axios interceptor showing a toast.
          skipErrorHandler: true,
        });
        const body = res?.data;
        if (!body?.success || !body.data) {
          throw new Error(body?.message || 'capacity request failed');
        }
        if (identityRef.current !== requestIdentity) return;
        loadedRef.current[scope] = true;
        loadedAtRef.current[scope] = Date.now();
        retryAfterRef.current[scope] = 0;
        if (scope === 'top') {
          setTopData(body.data);
          setTopError(false);
        } else {
          setAllData(body.data);
          setAllError(false);
        }
      } catch {
        if (identityRef.current !== requestIdentity) return;
        retryAfterRef.current[scope] = Date.now() + RETRY_COOLDOWN;
        if (scope === 'top') setTopError(true);
        else setAllError(true);
      } finally {
        if (identityRef.current !== requestIdentity) return;
        inFlightRef.current[scope] = false;
        if (scope === 'top') setTopLoading(false);
        else setAllLoading(false);
      }
    },
    [hasUser, identityKey],
  );

  useEffect(() => {
    if (initializedIdentityRef.current === identityKey) return;
    initializedIdentityRef.current = identityKey;
    identityRef.current = identityKey;
    loadedRef.current = { top: false, all: false };
    inFlightRef.current = { top: false, all: false };
    loadedAtRef.current = { top: 0, all: 0 };
    retryAfterRef.current = { top: 0, all: 0 };
    setTopData(null);
    setAllData(null);
    setTopError(false);
    setAllError(false);
    setGlobalExpanded(false);
    setGroupsExpanded(false);
    setGroupTotalsExpanded(false);
    setExpandedGroups({});
    if (hasUser) void loadCapacity('top');
  }, [identityKey, hasUser, loadCapacity]);

  const requestAll = useCallback(() => {
    void loadCapacity('all');
  }, [loadCapacity]);

  const toggleGlobal = () => {
    if (!globalExpanded) requestAll();
    setGlobalExpanded((value) => !value);
  };

  const toggleGroups = () => {
    if (!groupsExpanded) requestAll();
    setGroupsExpanded((value) => !value);
  };

  const toggleGroupTotals = () => {
    if (!groupTotalsExpanded) requestAll();
    setGroupTotalsExpanded((value) => !value);
  };

  const toggleGroup = (groupName) => {
    if (!expandedGroups[groupName]) requestAll();
    setExpandedGroups((value) => ({
      ...value,
      [groupName]: !value[groupName],
    }));
  };

  const anyExpanded = isAnyRateLimitCapacityExpanded({
    globalExpanded,
    groupsExpanded,
    groupTotalsExpanded,
    expandedGroups,
  });
  const refreshCapacity = () => {
    void loadCapacity('top', { force: true });
    if (anyExpanded) void loadCapacity('all', { force: true });
  };
  // The all snapshot is a source only while an area is expanded. Once
  // collapsed, return to the latest top snapshot for personal data and
  // metadata.
  // prettier-ignore
  const data = anyExpanded && allData ? allData : (topData || allData);
  const site = data?.site;
  const global = site?.global;
  const groups = site?.groups;
  const groupTotals = site?.group_totals;
  const globalItems = global?.items || [];
  const groupItems = groups?.groups || [];
  const displayedGlobalItems = globalExpanded
    ? globalItems
    : globalItems.slice(0, 3);
  const displayedGroups = groupsExpanded ? groupItems : groupItems.slice(0, 3);
  if (data && !data.site && !data.personal) return null;

  return (
    <Card
      className='mb-4 shadow-sm !rounded-2xl'
      title={
        <div className='flex items-center gap-2'>
          <Gauge size={16} />
          {t('RPM 概览')}
        </div>
      }
      bodyStyle={{ padding: 0 }}
    >
      {topLoading && !topData && (
        <div className='flex min-h-32 items-center justify-center'>
          <Spin spinning />
        </div>
      )}
      {topError && !topData && (
        <Banner
          type='warning'
          title={t('暂时不可用')}
          description={t('容量数据暂时不可用，请稍后重试。')}
        />
      )}
      {data?.degraded && (
        <Banner
          type='warning'
          title={t('容量数据部分不可用')}
          description={t('部分计数器暂时无法读取，不会将不可用数据显示为 0。')}
        />
      )}

      {anyExpanded && allLoading && (
        <div className='flex items-center gap-2 border-b border-gray-200 px-4 py-3 text-xs text-gray-500 sm:px-5'>
          <Spin size='small' spinning />
          {t('加载中...')}
        </div>
      )}
      {anyExpanded && allError && (
        <div className='border-b border-gray-200 px-4 py-3 text-xs text-gray-500 sm:px-5'>
          {t('暂时不可用')}
        </div>
      )}

      {global && (global.total > 0 || globalItems.length > 0) && (
        <section aria-label={t('全站模型 RPM')}>
          <CapacitySectionHeader
            title={t('全站模型 RPM')}
            windowLabel={t('固定 60 秒窗口')}
            total={global.total}
            displayedCount={displayedGlobalItems.length}
            expanded={globalExpanded}
            loading={allLoading}
            label='models'
            controlsId='classic-rate-limit-capacity-global'
            onIntent={requestAll}
            onToggle={toggleGlobal}
            t={t}
          />
          <div
            id='classic-rate-limit-capacity-global'
            className='grid gap-3 px-4 py-3 sm:px-5'
            style={{
              gridTemplateColumns: 'repeat(auto-fill, minmax(15rem, 1fr))',
            }}
          >
            {displayedGlobalItems.map((item) => (
              <CapacityRow
                key={`${item.model}:${item.group || ''}`}
                label={item.model}
                metric={item}
                t={t}
              />
            ))}
          </div>
        </section>
      )}

      {groups && (groups.total > 0 || groupItems.length > 0) && (
        <section aria-label={t('全站分组 RPM')}>
          <CapacitySectionHeader
            title={t('全站分组 RPM')}
            windowLabel={t('固定 60 秒窗口')}
            total={groups.total}
            displayedCount={displayedGroups.length}
            expanded={groupsExpanded}
            loading={allLoading}
            label='groups'
            controlsId='classic-rate-limit-capacity-groups'
            onIntent={requestAll}
            onToggle={toggleGroups}
            t={t}
          />
          <div
            id='classic-rate-limit-capacity-groups'
            className='grid gap-3 px-4 py-3 sm:px-5'
            style={{
              gridTemplateColumns: 'repeat(auto-fill, minmax(20rem, 1fr))',
            }}
          >
            {displayedGroups.map((group, index) => (
              <CapacityGroupCard
                key={group.group}
                group={group}
                index={index}
                expanded={Boolean(expandedGroups[group.group])}
                loading={allLoading}
                onIntent={requestAll}
                onToggle={() => toggleGroup(group.group)}
                t={t}
              />
            ))}
          </div>
        </section>
      )}

      {groupTotals && (
        <GroupTotalCapacitySection
          section={groupTotals}
          expanded={groupTotalsExpanded}
          loading={allLoading}
          onIntent={requestAll}
          onToggle={toggleGroupTotals}
          t={t}
        />
      )}

      {data?.personal && <PersonalSection personal={data.personal} t={t} />}
      {data && (
        <CapacityMetadata
          data={data}
          refreshing={topLoading || allLoading}
          onRefresh={refreshCapacity}
          t={t}
        />
      )}
    </Card>
  );
}
